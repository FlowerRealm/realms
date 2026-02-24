package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

func main() {
	var sqlitePath string
	var email string
	var username string
	var password string
	var rawToken string

	flag.StringVar(&sqlitePath, "sqlite-path", "", "SQLite path, e.g. ./data/realms.db?_busy_timeout=1000")
	flag.StringVar(&email, "email", "demo@example.com", "user email")
	flag.StringVar(&username, "username", "demo", "username")
	flag.StringVar(&password, "password", "demo123456", "password (plain, will be hashed)")
	flag.StringVar(&rawToken, "token", "sk_demo_usage_token", "user token (plain)")
	flag.Parse()

	sqlitePath = strings.TrimSpace(sqlitePath)
	if sqlitePath == "" {
		fmt.Fprintln(os.Stderr, "missing -sqlite-path")
		os.Exit(2)
	}

	basePath := sqlitePath
	if i := strings.Index(basePath, "?"); i >= 0 {
		basePath = basePath[:i]
	}
	if dir := filepath.Dir(basePath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}

	db, err := store.OpenSQLite(sqlitePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "OpenSQLite:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		fmt.Fprintln(os.Stderr, "EnsureSQLiteSchema:", err)
		os.Exit(1)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	ctx := context.Background()
	pwHash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "HashPassword:", err)
		os.Exit(1)
	}
	userID, err := st.CreateUser(ctx, email, username, pwHash, store.UserRoleUser)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CreateUser:", err)
		os.Exit(1)
	}
	tokenName := "demo-token"
	tokenID, _, err := st.CreateUserToken(ctx, userID, &tokenName, rawToken)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CreateUserToken:", err)
		os.Exit(1)
	}

	now := time.Now().UTC()
	newUsageEvent := func(reqID string, endpoint string, inTok, outTok int64, committedUSD string) {
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           userID,
			SubscriptionID:   nil,
			TokenID:          tokenID,
			Model:            func() *string { s := "gpt-5.2"; return &s }(),
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "ReserveUsage:", err)
			os.Exit(1)
		}
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID: usageID,
			InputTokens:  &inTok,
			OutputTokens: &outTok,
			CommittedUSD: decimal.RequireFromString(committedUSD),
		}); err != nil {
			fmt.Fprintln(os.Stderr, "CommitUsage:", err)
			os.Exit(1)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID: usageID,
			Endpoint:     endpoint,
			Method:       "POST",
			StatusCode:   200,
			LatencyMS:    120,
			IsStream:     false,
			RequestBytes:  1234,
			ResponseBytes: 5678,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "FinalizeUsageEvent:", err)
			os.Exit(1)
		}
	}

	newUsageEvent("req_demo_1", "/v1/responses", 120, 45, "0.42")
	newUsageEvent("req_demo_2", "/v1/chat/completions", 30, 10, "0.08")
	newUsageEvent("req_demo_3", "/v1/messages", 200, 90, "0.77")

	fmt.Printf("seeded sqlite=%s user_id=%d token_id=%d token=%s\n", sqlitePath, userID, tokenID, rawToken)
}

