package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/logging"
	"github.com/ebenderooock/loom/internal/storage"
)

func cmdAPIKey(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: loom api-key <create|list|revoke> [flags]")
	}
	switch args[0] {
	case "create":
		return cmdAPIKeyCreate(ctx, args[1:])
	case "list":
		return cmdAPIKeyList(ctx, args[1:])
	case "revoke":
		return cmdAPIKeyRevokeWithUser(ctx, args[1:])
	default:
		return fmt.Errorf("unknown api-key subcommand %q", args[0])
	}
}

func cmdAPIKeyCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("api-key create", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml")
	user := fs.String("user", "", "username (required)")
	name := fs.String("name", "", "label for the key (required)")
	expires := fs.Duration("expires", 0, "expiry duration (e.g. 720h); zero means never")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *user == "" || *name == "" {
		return errors.New("--user and --name are required")
	}
	store, _, _, closer, err := openCLIStore(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closer()

	u, err := store.GetUserByUsername(ctx, *user)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		return err
	}
	var expiresAt *time.Time
	if *expires > 0 {
		t := time.Now().Add(*expires)
		expiresAt = &t
	}
	k, err := store.CreateAPIKey(ctx, auth.CreateAPIKeyParams{
		UserID:    u.ID,
		Name:      *name,
		KeyHash:   hash,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	fmt.Printf("api key created (id=%d, prefix=%s). Store the key below — it will not be shown again:\n", k.ID, k.Prefix)
	fmt.Println(key)
	return nil
}

func cmdAPIKeyList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("api-key list", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml")
	user := fs.String("user", "", "username (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *user == "" {
		return errors.New("--user is required")
	}
	store, _, _, closer, err := openCLIStore(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closer()

	u, err := store.GetUserByUsername(ctx, *user)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	keys, err := store.ListAPIKeysForUser(ctx, u.ID)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		fmt.Println("no keys")
		return nil
	}
	fmt.Printf("%-6s %-20s %-12s %-25s %-25s\n", "ID", "NAME", "PREFIX", "CREATED", "EXPIRES")
	for _, k := range keys {
		exp := "never"
		if k.ExpiresAt != nil {
			exp = k.ExpiresAt.Format(time.RFC3339)
		}
		fmt.Printf("%-6d %-20s %-12s %-25s %-25s\n", k.ID, k.Name, k.Prefix, k.CreatedAt.Format(time.RFC3339), exp)
	}
	return nil
}

func cmdAPIKeyRevokeWithUser(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("api-key revoke", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml")
	id := fs.Int64("id", 0, "api key ID (required)")
	user := fs.String("user", "", "username owning the key (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == 0 || *user == "" {
		return errors.New("--id and --user are required")
	}
	store, _, _, closer, err := openCLIStore(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closer()
	u, err := store.GetUserByUsername(ctx, *user)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	if err := store.RevokeAPIKey(ctx, *id, u.ID); err != nil {
		return err
	}
	fmt.Printf("revoked api key id=%d for user %s\n", *id, *user)
	return nil
}

func cmdUser(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: loom user <create|passwd> [flags]")
	}
	switch args[0] {
	case "create":
		return cmdUserCreate(ctx, args[1:])
	case "passwd":
		return cmdUserPasswd(ctx, args[1:])
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func cmdUserCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user create", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml")
	username := fs.String("username", "", "username (required)")
	email := fs.String("email", "", "email")
	role := fs.String("role", "user", "role (admin|user)")
	password := fs.String("password", "", "password (prompted if omitted)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" {
		return errors.New("--username is required")
	}
	if *role != "admin" && *role != "user" {
		return fmt.Errorf("--role must be admin|user, got %q", *role)
	}
	pw := *password
	if pw == "" {
		got, err := promptPassword("Password: ")
		if err != nil {
			return err
		}
		pw = got
	}
	store, _, _, closer, err := openCLIStore(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closer()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	u, err := store.CreateUser(ctx, auth.CreateUserParams{
		Username:     *username,
		PasswordHash: hash,
		Email:        *email,
		Role:         *role,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	fmt.Printf("user created: id=%d username=%s role=%s\n", u.ID, u.Username, u.Role)
	return nil
}

func cmdUserPasswd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user passwd", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml")
	username := fs.String("username", "", "username (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" {
		return errors.New("--username is required")
	}
	pw, err := promptPassword("New password: ")
	if err != nil {
		return err
	}
	store, _, _, closer, err := openCLIStore(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closer()
	u, err := store.GetUserByUsername(ctx, *username)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := store.UpdateUserPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	fmt.Printf("password updated for user %s\n", *username)
	return nil
}

// openCLIStore builds an auth.Store on top of a freshly-opened storage
// connection that has been migrated. Returns a closer the caller must run.
func openCLIStore(ctx context.Context, configPath string) (auth.Store, *config.Config, storage.DB, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, nil, func() {}, fmt.Errorf("load config: %w", err)
	}
	logger, err := logging.New(cfg.Log)
	if err != nil {
		return nil, nil, nil, func() {}, fmt.Errorf("init logger: %w", err)
	}
	db, err := storage.Open(ctx, cfg.Storage, logger)
	if err != nil {
		return nil, nil, nil, func() {}, fmt.Errorf("open storage: %w", err)
	}
	if err := db.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, nil, nil, func() {}, fmt.Errorf("migrate: %w", err)
	}
	store, err := auth.StoreFromDB(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, func() {}, err
	}
	return store, cfg, db, func() { _ = db.Close() }, nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
