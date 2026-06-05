package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ImportProwlarr reads indexer configurations from a Prowlarr SQLite database
// and inserts them into Loom's indexers table.
func (imp *Importer) ImportProwlarr(ctx context.Context, prowlarrDBPath string) (*ImportResult, error) {
	start := time.Now()
	res := &ImportResult{Source: "prowlarr"}

	src, err := openSourceDB(prowlarrDBPath)
	if err != nil {
		return nil, fmt.Errorf("open prowlarr db: %w", err)
	}
	defer src.Close()

	if err := src.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping prowlarr db: %w", err)
	}

	tx, err := imp.loomDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	imp.importProwlarrIndexers(ctx, src, tx, res)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	res.Duration = time.Since(start)
	return res, nil
}

// prowlarrSettings is a partial parse of the Prowlarr Indexers.Settings JSON.
type prowlarrSettings struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	APIURL  string `json:"apiPath"`
}

func (imp *Importer) importProwlarrIndexers(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx,
		`SELECT Id, Name, COALESCE(Implementation, ''), COALESCE(Settings, '{}'),
		        COALESCE(Enable, 1), COALESCE(Priority, 25), COALESCE(Protocol, 1)
		 FROM Indexers`)
	if err != nil {
		imp.logger.Warn("prowlarr: could not read Indexers", "err", err)
		res.Errors = append(res.Errors, "read Indexers: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id           int64
			name, impl   string
			settingsJSON string
			enabled      int
			priority     int
			protocol     int
		)
		if err := rows.Scan(&id, &name, &impl, &settingsJSON, &enabled, &priority, &protocol); err != nil {
			res.Errors = append(res.Errors, "scan indexer: "+err.Error())
			continue
		}

		var settings prowlarrSettings
		_ = json.Unmarshal([]byte(settingsJSON), &settings)

		lid := loomID("prowlarr", id)
		protoStr := "usenet"
		if protocol == 2 {
			protoStr = "torrent"
		}

		enabledInt := 0
		if enabled != 0 {
			enabledInt = 1
		}

		// Loom's indexers table: id, kind, name, enabled, priority, config_json
		configJSON, _ := json.Marshal(map[string]string{
			"url":            settings.BaseURL,
			"apiKey":         settings.APIKey,
			"implementation": impl,
		})

		result, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO indexers
			 (id, kind, name, enabled, priority, config_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			lid, protoStr, name, enabledInt, priority, string(configJSON))
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert indexer %q: %s", name, err))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			res.IndexersAdded++
			imp.logger.Info("prowlarr: imported indexer", "name", name, "id", lid)
		} else {
			res.Skipped++
		}
	}
}
