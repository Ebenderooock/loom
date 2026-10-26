package indexers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"
)

// ProviderBudget is an explicitly named provider/account allowance. IDs must
// be operator-assigned labels, never credentials or inferred hostnames.
type ProviderBudget struct {
	ID                string   `json:"id"`
	Allowance         int64    `json:"allowance"`
	WindowSeconds     int64    `json:"window_seconds"`
	Consumed          int64    `json:"consumed"`
	ResetAt           int64    `json:"reset_at"`
	ProviderRemaining *int64   `json:"provider_remaining"`
	ProviderResetAt   int64    `json:"provider_reset_at"`
	IndexerIDs        []string `json:"indexer_ids"`
}

// BudgetDenied is a normal admission skip, not a circuit-breaker failure.
type BudgetDenied struct {
	Reason  string
	ResetAt int64
}

func (e *BudgetDenied) Error() string {
	if e.ResetAt == 0 {
		return e.Reason + "; resume time unknown (provider reset required)"
	}
	return fmt.Sprintf("%s; resumes at %s", e.Reason, time.Unix(e.ResetAt, 0).UTC().Format(time.RFC3339))
}

// Retryable prevents transport retries from retrying a denied admission.
func (e *BudgetDenied) Retryable() bool { return false }

type budgetAccountingError struct{ err error }

func (e *budgetAccountingError) Error() string {
	return "provider budget accounting unavailable; search blocked"
}
func (e *budgetAccountingError) Unwrap() error   { return e.err }
func (e *budgetAccountingError) Retryable() bool { return false }

func accountingError(err error) error {
	slog.Error("provider budget accounting failed", "error", err)
	return &budgetAccountingError{err: err}
}

// ProviderBudgetStore persists accounting using SQL conditional updates.
type ProviderBudgetStore struct {
	db    *sql.DB
	clock Clock
}

// NewProviderBudgetStore supports both SQLite and PostgreSQL.
func NewProviderBudgetStore(db *sql.DB, clock Clock) *ProviderBudgetStore {
	if clock == nil {
		clock = SystemClock{}
	}
	return &ProviderBudgetStore{db: db, clock: clock}
}

var budgetLabel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Configure replaces explicit group membership without clearing accounting.
// Existing membership in another group is rejected, rather than silently moved.
func (s *ProviderBudgetStore) Configure(ctx context.Context, b ProviderBudget) error {
	if !budgetLabel.MatchString(b.ID) || b.Allowance < 1 || b.Allowance > 1_000_000_000 ||
		b.WindowSeconds < 1 || b.WindowSeconds > 366*24*60*60 {
		return errors.New("budget requires an opaque label, positive allowance (<= 1000000000), and window_seconds (<= 31622400)")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_budgets (id, allowance, window_seconds, reset_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET
		allowance=excluded.allowance, window_seconds=excluded.window_seconds`,
		b.ID, b.Allowance, b.WindowSeconds, s.clock.Now().Unix()+b.WindowSeconds)
	if err != nil {
		return fmt.Errorf("configure provider budget: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM provider_budget_members WHERE budget_id=$1`, b.ID); err != nil {
		return err
	}
	seen := make(map[string]bool, len(b.IndexerIDs))
	for _, id := range b.IndexerIDs {
		if seen[id] {
			return errors.New("duplicate indexer in provider budget")
		}
		seen[id] = true
		if _, err = tx.ExecContext(ctx, `INSERT INTO provider_budget_members (indexer_id,budget_id) VALUES ($1,$2)`, id, b.ID); err != nil {
			return fmt.Errorf("assign provider budget member (must exist and not belong to another group): %w", err)
		}
	}
	return tx.Commit()
}

const budgetColumns = `id, allowance, window_seconds, consumed, reset_at, provider_remaining, provider_reset_at`

type budgetScanner interface {
	Scan(...any) error
}

func scanBudget(row budgetScanner) (ProviderBudget, error) {
	var b ProviderBudget
	var remaining sql.NullInt64
	err := row.Scan(&b.ID, &b.Allowance, &b.WindowSeconds, &b.Consumed, &b.ResetAt, &remaining, &b.ProviderResetAt)
	if remaining.Valid {
		b.ProviderRemaining = &remaining.Int64
	}
	b.IndexerIDs = []string{}
	return b, err
}

// List exposes persisted state and explicit membership, never provider credentials.
func (s *ProviderBudgetStore) List(ctx context.Context) ([]ProviderBudget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+budgetColumns+` FROM provider_budgets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	budgets := []ProviderBudget{}
	for rows.Next() {
		b, scanErr := scanBudget(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		budgets = append(budgets, b)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range budgets {
		members, queryErr := s.db.QueryContext(ctx, `SELECT indexer_id FROM provider_budget_members WHERE budget_id=$1 ORDER BY indexer_id`, budgets[i].ID)
		if queryErr != nil {
			return nil, queryErr
		}
		for members.Next() {
			var id string
			if err = members.Scan(&id); err != nil {
				_ = members.Close()
				return nil, err
			}
			budgets[i].IndexerIDs = append(budgets[i].IndexerIDs, id)
		}
		err = members.Err()
		_ = members.Close()
		if err != nil {
			return nil, err
		}
	}
	return budgets, nil
}

// Reserve atomically consumes one attempt, including failed attempts. No
// read-modify-write transaction or in-process lock governs admission.
func (s *ProviderBudgetStore) Reserve(ctx context.Context, indexerID string) error {
	_, err := s.reserve(ctx, indexerID)
	return err
}

func (s *ProviderBudgetStore) reserve(ctx context.Context, indexerID string) (*ProviderBudget, error) {
	now := s.clock.Now().Unix()
	row := s.db.QueryRowContext(ctx, `UPDATE provider_budgets SET
		consumed=CASE WHEN reset_at <= $2 THEN 1 ELSE consumed+1 END,
		reset_at=CASE WHEN reset_at <= $2 THEN $2+window_seconds ELSE reset_at END,
		provider_remaining=CASE WHEN provider_reset_at > 0 AND provider_reset_at <= $2 THEN NULL
			WHEN provider_remaining IS NOT NULL THEN provider_remaining-1 ELSE NULL END,
		provider_reset_at=CASE WHEN provider_reset_at > 0 AND provider_reset_at <= $2 THEN 0 ELSE provider_reset_at END
		WHERE id=(SELECT budget_id FROM provider_budget_members WHERE indexer_id=$1)
		AND (reset_at <= $2 OR consumed < allowance)
		AND (provider_remaining IS NULL OR provider_remaining > 0 OR
			(provider_reset_at > 0 AND provider_reset_at <= $2))
		RETURNING `+budgetColumns, indexerID, now)
	reserved, err := scanBudget(row)
	if err == nil {
		return &reserved, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, accountingError(err)
	}
	b, err := scanBudget(s.db.QueryRowContext(ctx, `SELECT `+budgetColumns+` FROM provider_budgets
		WHERE id=(SELECT budget_id FROM provider_budget_members WHERE indexer_id=$1)`, indexerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // Unconfigured definitions retain their existing admission policy.
	}
	if err != nil {
		return nil, accountingError(err)
	}
	reset := int64(0)
	if b.Consumed >= b.Allowance && b.ResetAt > now {
		reset = b.ResetAt
	}
	reason := "configured provider allowance exhausted"
	if b.ProviderRemaining != nil && *b.ProviderRemaining <= 0 && (b.ProviderResetAt == 0 || b.ProviderResetAt > now) {
		reason = "provider-reported allowance exhausted"
		if b.ProviderResetAt == 0 || b.ProviderResetAt > reset {
			reset = b.ProviderResetAt
		}
	}
	return nil, &BudgetDenied{Reason: reason, ResetAt: reset}
}

// observe only tightens a live allowance, subtracting reservations made since
// this attempt to avoid granting calls from stale, out-of-order responses.
func (s *ProviderBudgetStore) observe(ctx context.Context, ticket ProviderBudget, remaining, reset int64) error {
	now := s.clock.Now().Unix()
	_, err := s.db.ExecContext(ctx, `UPDATE provider_budgets SET
		provider_remaining=CASE
			WHEN CAST($2 AS BIGINT)=0 THEN 0
			WHEN provider_remaining IS NULL OR (provider_reset_at > 0 AND provider_reset_at <= $4)
				OR $2-(consumed-$5) < provider_remaining THEN
				CASE WHEN $2 > consumed-$5 THEN $2-(consumed-$5) ELSE 0 END
			ELSE provider_remaining END,
		provider_reset_at=CASE
			WHEN provider_remaining IS NULL OR (provider_reset_at > 0 AND provider_reset_at <= $4) THEN CAST($3 AS BIGINT)
			WHEN provider_reset_at=0 OR $3=0 THEN 0
			WHEN $3 > provider_reset_at THEN $3 ELSE provider_reset_at END
		WHERE id=$1 AND (reset_at=$6 OR $2=0)`,
		ticket.ID, remaining, reset, now, ticket.Consumed, ticket.ResetAt)
	if err != nil {
		return accountingError(err)
	}
	return nil
}
