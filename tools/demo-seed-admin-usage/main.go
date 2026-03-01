package main

import (
	"context"
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

func main() {
	_ = godotenv.Load()

	var user1Email string
	var user1Username string
	var user2Email string
	var user2Username string
	var password string

	flag.StringVar(&user1Email, "user1-email", "review1@example.com", "user1 email")
	flag.StringVar(&user1Username, "user1-username", "review1", "user1 username (letters/digits only)")
	flag.StringVar(&user2Email, "user2-email", "review2@example.com", "user2 email")
	flag.StringVar(&user2Username, "user2-username", "review2", "user2 username (letters/digits only)")
	flag.StringVar(&password, "password", "review123456", "password for both users (plain, will be hashed)")
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

	createOrGetUser := func(email, username, role string) (store.User, error) {
		email = strings.TrimSpace(strings.ToLower(email))
		username = strings.TrimSpace(username)
		if email == "" || username == "" {
			return store.User{}, fmt.Errorf("email/username 不能为空")
		}
		id, err := st.CreateUser(ctx, email, username, pwHash, role)
		if err == nil && id > 0 {
			return st.GetUserByID(ctx, id)
		}
		u, err2 := st.GetUserByEmail(ctx, email)
		if err2 == nil && u.ID > 0 {
			return u, nil
		}
		return store.User{}, err
	}

	user1, err := createOrGetUser(user1Email, user1Username, store.UserRoleUser)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Create/Get user1:", err)
		os.Exit(1)
	}
	user2, err := createOrGetUser(user2Email, user2Username, store.UserRoleUser)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Create/Get user2:", err)
		os.Exit(1)
	}

	tokenName1 := "review1-key"
	tokenName2 := "review2-key"
	rawToken1 := "sk_review1_seed"
	rawToken2 := "sk_review2_seed"
	getOrCreateToken := func(userID int64, name string, raw string) (int64, string, error) {
		n := strings.TrimSpace(name)
		if n == "" {
			n = "seed-key"
		}
		tokens, err := st.ListUserTokens(ctx, userID)
		if err != nil {
			return 0, "", err
		}
		for _, tok := range tokens {
			if tok.ID <= 0 || tok.Name == nil {
				continue
			}
			if strings.TrimSpace(*tok.Name) == n {
				return tok.ID, "", nil
			}
		}

		id, _, err := st.CreateUserToken(ctx, userID, &n, raw)
		if err != nil {
			return 0, "", err
		}
		return id, raw, nil
	}

	token1, tokenPlain1, err := getOrCreateToken(user1.ID, tokenName1, rawToken1)
	if err != nil || token1 <= 0 {
		fmt.Fprintln(os.Stderr, "Create/Get token1:", err)
		os.Exit(1)
	}
	token2, tokenPlain2, err := getOrCreateToken(user2.ID, tokenName2, rawToken2)
	if err != nil || token2 <= 0 {
		fmt.Fprintln(os.Stderr, "Create/Get token2:", err)
		os.Exit(1)
	}

	getOrCreateChannel := func(name string) (int64, error) {
		name = strings.TrimSpace(name)
		if name == "" {
			return 0, fmt.Errorf("channel name 不能为空")
		}
		channels, err2 := st.ListUpstreamChannels(ctx)
		if err2 != nil {
			return 0, err2
		}
		for _, ch := range channels {
			if strings.TrimSpace(ch.Name) == name {
				return ch.ID, nil
			}
		}
		id, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, name, "", 0, false, false, false, false)
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	chA, err := getOrCreateChannel("seed-channel-A")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Create/Get upstream channel A:", err)
		os.Exit(1)
	}
	chB, err := getOrCreateChannel("seed-channel-B")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Create/Get upstream channel B:", err)
		os.Exit(1)
	}

	now := time.Now().UTC()
	mkEvent := func(userID, tokenID, chID int64, requestID, endpoint, model string, inTok, outTok int64, usd string, status int) error {
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           userID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			return fmt.Errorf("ReserveUsage(%s): %w", requestID, err)
		}
		chRef := chID
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			InputTokens:       &inTok,
			OutputTokens:      &outTok,
			CommittedUSD:      decimal.RequireFromString(usd),
		}); err != nil {
			return fmt.Errorf("CommitUsage(%s): %w", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:        usageID,
			Endpoint:            endpoint,
			Method:              "POST",
			StatusCode:          status,
			LatencyMS:           420,
			FirstTokenLatencyMS: 120,
			UpstreamChannelID:   &chRef,
			IsStream:            false,
			RequestBytes:        1234,
			ResponseBytes:       5678,
		}); err != nil {
			return fmt.Errorf("FinalizeUsageEvent(%s): %w", requestID, err)
		}
		return nil
	}

	// Insert a few deterministic “fake requests” for review: user/model/channel mixes.
	if err := mkEvent(user1.ID, token1, chA, "req_seed_u1_gpt_1", "/v1/responses", "gpt-5.2", 1200, 300, "0.42", 200); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := mkEvent(user1.ID, token1, chB, "req_seed_u1_claude_1", "/v1/chat/completions", "claude-3", 800, 220, "0.31", 200); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := mkEvent(user2.ID, token2, chA, "req_seed_u2_gpt_1", "/v1/responses", "gpt-5.2", 500, 120, "0.19", 200); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := mkEvent(user2.ID, token2, chB, "req_seed_u2_gpt_2", "/v1/messages", "gpt-4.1", 900, 260, "0.55", 502); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("seeded users:\n")
	fmt.Printf("- user1 id=%d email=%s username=%s token_id=%d token=%s\n", user1.ID, user1.Email, user1.Username, token1, tokenPlain1)
	fmt.Printf("- user2 id=%d email=%s username=%s token_id=%d token=%s\n", user2.ID, user2.Email, user2.Username, token2, tokenPlain2)
	fmt.Printf("seeded channels: %d(seed-channel-A), %d(seed-channel-B)\n", chA, chB)
	fmt.Printf("seeded requests: req_seed_u1_gpt_1 req_seed_u1_claude_1 req_seed_u2_gpt_1 req_seed_u2_gpt_2\n")
}
