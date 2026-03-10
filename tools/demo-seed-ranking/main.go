package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/store"
)

type seedUser struct {
	email    string
	username string
	token    string
	requests []seedRequest
}

type seedRequest struct {
	requestID string
	at        time.Time
	model     string
	usd       string
	inputTok  int64
	outputTok int64
}

func main() {
	_ = godotenv.Load()

	var password string
	flag.StringVar(&password, "password", "demo123456", "password for demo users")
	flag.Parse()

	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "LoadFromEnv:", err)
		os.Exit(1)
	}

	db, dialect, err := store.OpenDB(cfg.Env, cfg.DB.Driver, cfg.DB.DSN, cfg.DB.SQLitePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "OpenDB:", err)
		os.Exit(1)
	}
	defer db.Close()

	switch dialect {
	case store.DialectMySQL:
		if err := store.ApplyMigrations(context.Background(), db, store.MigrationOptions{
			LockName:    cfg.DB.MigrationLockName,
			LockTimeout: time.Duration(cfg.DB.MigrationLockTimeoutSeconds) * time.Second,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "ApplyMigrations:", err)
			os.Exit(1)
		}
	case store.DialectSQLite:
		if err := store.EnsureSQLiteSchema(db); err != nil {
			fmt.Fprintln(os.Stderr, "EnsureSQLiteSchema:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown dialect:", dialect)
		os.Exit(1)
	}

	st := store.New(db)
	st.SetDialect(dialect)
	ctx := context.Background()

	pwHash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "HashPassword:", err)
		os.Exit(1)
	}

	now := time.Now().UTC()
	users := []seedUser{
		{
			email:    "nova@example.com",
			username: "nova",
			token:    "sk_demo_nova",
			requests: []seedRequest{
				{requestID: "rk_nova_1d_1", at: now.Add(-2 * time.Hour), model: "gpt-5.2", usd: "88", inputTok: 8000, outputTok: 2200},
				{requestID: "rk_nova_7d_1", at: now.Add(-2 * 24 * time.Hour), model: "gpt-5.2", usd: "24", inputTok: 4000, outputTok: 1200},
				{requestID: "rk_nova_1mo_1", at: now.Add(-18 * 24 * time.Hour), model: "claude-3", usd: "16", inputTok: 2200, outputTok: 900},
			},
		},
		{
			email:    "atlas@example.com",
			username: "atlas",
			token:    "sk_demo_atlas",
			requests: []seedRequest{
				{requestID: "rk_atlas_1d_1", at: now.Add(-8 * time.Hour), model: "gpt-4.1", usd: "53", inputTok: 6200, outputTok: 1900},
				{requestID: "rk_atlas_7d_1", at: now.Add(-5 * 24 * time.Hour), model: "gpt-4.1", usd: "44", inputTok: 5000, outputTok: 1600},
				{requestID: "rk_atlas_1mo_1", at: now.Add(-25 * 24 * time.Hour), model: "gpt-4.1", usd: "11", inputTok: 1800, outputTok: 600},
			},
		},
		{
			email:    "echo@example.com",
			username: "echo",
			token:    "sk_demo_echo",
			requests: []seedRequest{
				{requestID: "rk_echo_7d_1", at: now.Add(-3 * 24 * time.Hour), model: "gemini-2.5-pro", usd: "97", inputTok: 9000, outputTok: 2800},
				{requestID: "rk_echo_1mo_1", at: now.Add(-21 * 24 * time.Hour), model: "gemini-2.5-pro", usd: "21", inputTok: 2400, outputTok: 850},
			},
		},
		{
			email:    "ember@example.com",
			username: "ember",
			token:    "sk_demo_ember",
			requests: []seedRequest{
				{requestID: "rk_ember_1mo_1", at: now.Add(-10 * 24 * time.Hour), model: "deepseek-v3", usd: "140", inputTok: 13000, outputTok: 4200},
			},
		},
	}

	channelID, err := getOrCreateChannel(ctx, st, "ranking-demo-channel")
	if err != nil {
		fmt.Fprintln(os.Stderr, "getOrCreateChannel:", err)
		os.Exit(1)
	}

	for _, candidate := range users {
		user, err := createOrGetUser(ctx, st, candidate.email, candidate.username, pwHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Create/Get user %s: %v\n", candidate.email, err)
			os.Exit(1)
		}
		tokenID, err := getOrCreateToken(ctx, st, user.ID, candidate.username+"-key", candidate.token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Create/Get token %s: %v\n", candidate.email, err)
			os.Exit(1)
		}
		for _, req := range candidate.requests {
			if err := createUsageEvent(ctx, db, st, user.ID, tokenID, channelID, req); err != nil {
				fmt.Fprintf(os.Stderr, "Create usage %s: %v\n", req.requestID, err)
				os.Exit(1)
			}
		}
	}

	fmt.Printf("seeded ranking demo users=%d password=%s login=%s\n", len(users), password, users[0].email)
}

func createOrGetUser(ctx context.Context, st *store.Store, email, username string, pwHash []byte) (store.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	username = strings.TrimSpace(username)
	if email == "" || username == "" {
		return store.User{}, fmt.Errorf("email/username 不能为空")
	}
	id, err := st.CreateUser(ctx, email, username, pwHash, store.UserRoleUser)
	if err == nil && id > 0 {
		return st.GetUserByID(ctx, id)
	}
	user, getErr := st.GetUserByEmail(ctx, email)
	if getErr == nil && user.ID > 0 {
		return user, nil
	}
	return store.User{}, err
}

func getOrCreateToken(ctx context.Context, st *store.Store, userID int64, name, raw string) (int64, error) {
	tokens, err := st.ListUserTokens(ctx, userID)
	if err != nil {
		return 0, err
	}
	name = strings.TrimSpace(name)
	for _, tok := range tokens {
		if tok.ID > 0 && tok.Name != nil && strings.TrimSpace(*tok.Name) == name {
			return tok.ID, nil
		}
	}
	id, _, err := st.CreateUserToken(ctx, userID, &name, raw)
	return id, err
}

func getOrCreateChannel(ctx context.Context, st *store.Store, name string) (int64, error) {
	channels, err := st.ListUpstreamChannels(ctx)
	if err != nil {
		return 0, err
	}
	for _, ch := range channels {
		if strings.TrimSpace(ch.Name) == name {
			return ch.ID, nil
		}
	}
	return st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, name, "", 0, false, false, false, false)
}

func createUsageEvent(ctx context.Context, db *sql.DB, st *store.Store, userID, tokenID, channelID int64, req seedRequest) error {
	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        req.requestID,
		UserID:           userID,
		TokenID:          tokenID,
		Model:            &req.model,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: req.at.Add(time.Hour),
	})
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return nil
	}
	if err != nil {
		return err
	}
	channelRef := channelID
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:      usageID,
		UpstreamChannelID: &channelRef,
		InputTokens:       &req.inputTok,
		OutputTokens:      &req.outputTok,
		CommittedUSD:      decimal.RequireFromString(req.usd),
	}); err != nil {
		return err
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:        usageID,
		Endpoint:            "/v1/responses",
		Method:              "POST",
		StatusCode:          200,
		LatencyMS:           480,
		FirstTokenLatencyMS: 120,
		UpstreamChannelID:   &channelRef,
		IsStream:            false,
		RequestBytes:        2048,
		ResponseBytes:       4096,
	}); err != nil {
		return err
	}
	ts := req.at.UTC().Format("2006-01-02 15:04:05")
	_, err = db.ExecContext(ctx, `UPDATE usage_events SET time=?, created_at=?, updated_at=? WHERE request_id=?`, ts, ts, ts, req.requestID)
	return err
}
