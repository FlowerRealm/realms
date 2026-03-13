package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

type recordedUpstreamCall struct {
	Model string
}

type runtimeInfoE2E struct {
	Available    bool `json:"available"`
	FailScore    int  `json:"fail_score"`
	BannedActive bool `json:"banned_active"`
}

func TestResponses_ChannelModelFailureScopesToBinding_E2E(t *testing.T) {
	const (
		badModel  = "alias-bad"
		goodModel = "alias-good"

		badUpstreamModel  = "bad-upstream"
		goodUpstreamModel = "good-upstream"

		userGroup  = "ug1"
		routeGroup = "rg1"
	)

	writeSuccess := func(w http.ResponseWriter, model string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_" + strings.ReplaceAll(model, "-", "_"),
			"object": "response",
			"model":  model,
			"output": []any{
				map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": "OK"},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
			"status": "completed",
		})
	}

	var (
		mu            sync.Mutex
		primaryCalls  []recordedUpstreamCall
		fallbackCalls []recordedUpstreamCall
	)
	recordCall := func(dst *[]recordedUpstreamCall, body []byte) string {
		model := extractForwardedModelE2E(body)
		mu.Lock()
		*dst = append(*dst, recordedUpstreamCall{Model: model})
		mu.Unlock()
		return model
	}
	assertCalls := func(label string, got []recordedUpstreamCall, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s 调用次数不对: got=%d want=%d calls=%v", label, len(got), len(want), got)
		}
		for i := range want {
			if got[i].Model != want[i] {
				t.Fatalf("%s 第 %d 次模型不对: got=%q want=%q calls=%v", label, i+1, got[i].Model, want[i], got)
			}
		}
	}
	assertRecordedCalls := func(wantPrimary []string, wantFallback []string) {
		t.Helper()
		mu.Lock()
		gotPrimary := append([]recordedUpstreamCall(nil), primaryCalls...)
		gotFallback := append([]recordedUpstreamCall(nil), fallbackCalls...)
		mu.Unlock()

		assertCalls("primary", gotPrimary, wantPrimary)
		assertCalls("fallback", gotFallback, wantFallback)
	}

	primaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		model := recordCall(&primaryCalls, body)

		switch model {
		case badUpstreamModel:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "model_not_found",
					"message": "model bad-upstream not found",
				},
			})
		case goodUpstreamModel:
			writeSuccess(w, model)
		default:
			t.Fatalf("primary upstream 收到意外模型: %q body=%s", model, string(body))
		}
	}))
	t.Cleanup(primaryUpstream.Close)

	fallbackUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		model := recordCall(&fallbackCalls, body)

		switch model {
		case badUpstreamModel, goodUpstreamModel:
			writeSuccess(w, model)
		default:
			t.Fatalf("fallback upstream 收到意外模型: %q body=%s", model, string(body))
		}
	}))
	t.Cleanup(fallbackUpstream.Close)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)
	ctx := context.Background()

	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	if err := st.CreateMainGroup(ctx, userGroup, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, userGroup, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	primaryChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "primary", routeGroup, 100, true, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(primary): %v", err)
	}
	primaryEndpointID, err := st.CreateUpstreamEndpoint(ctx, primaryChannelID, strings.TrimRight(strings.TrimSpace(primaryUpstream.URL), "/")+"/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint(primary): %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, primaryEndpointID, strPtr("primary"), "sk-primary"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(primary): %v", err)
	}

	fallbackChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "fallback", routeGroup, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(fallback): %v", err)
	}
	fallbackEndpointID, err := st.CreateUpstreamEndpoint(ctx, fallbackChannelID, strings.TrimRight(strings.TrimSpace(fallbackUpstream.URL), "/")+"/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint(fallback): %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, fallbackEndpointID, strPtr("fallback"), "sk-fallback"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(fallback): %v", err)
	}

	for _, publicModel := range []string{badModel, goodModel} {
		if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
			PublicID:            publicModel,
			GroupName:           routeGroup,
			OwnedBy:             strPtr("upstream"),
			InputUSDPer1M:       decimal.RequireFromString("1"),
			OutputUSDPer1M:      decimal.RequireFromString("1"),
			CacheInputUSDPer1M:  decimal.Zero,
			CacheOutputUSDPer1M: decimal.Zero,
			Status:              1,
		}); err != nil {
			t.Fatalf("CreateManagedModel(%s): %v", publicModel, err)
		}
	}

	primaryBadBindingID, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     primaryChannelID,
		PublicID:      badModel,
		UpstreamModel: badUpstreamModel,
		Status:        1,
	})
	if err != nil {
		t.Fatalf("CreateChannelModel(primary bad): %v", err)
	}
	primaryGoodBindingID, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     primaryChannelID,
		PublicID:      goodModel,
		UpstreamModel: goodUpstreamModel,
		Status:        1,
	})
	if err != nil {
		t.Fatalf("CreateChannelModel(primary good): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     fallbackChannelID,
		PublicID:      badModel,
		UpstreamModel: badUpstreamModel,
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(fallback bad): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     fallbackChannelID,
		PublicID:      goodModel,
		UpstreamModel: goodUpstreamModel,
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(fallback good): %v", err)
	}

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootUserID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}

	userID, err := st.CreateUser(ctx, "binding-scope@example.com", "bindingscope", []byte("x"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(user): %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, userGroup); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("1")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}
	rawToken, err := auth.NewRandomToken("sk_", 32)
	if err != nil {
		t.Fatalf("NewRandomToken: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, strPtr("binding-scope-token"), rawToken)
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("SESSION_SECRET", "e2e-test-secret")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.Mode = config.ModeBusiness
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.Billing.EnablePayAsYouGo = true

	app, err := server.NewApp(server.AppOptions{
		Config:  appCfg,
		DB:      db,
		Version: version.Info(),
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)

	client := &http.Client{Timeout: 10 * time.Second}
	doResponses := func(model string) {
		t.Helper()

		reqBody := []byte(`{"model":"` + model + `","input":"hi","stream":false}`)
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("NewRequest(%s): %v", model, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+rawToken)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do(%s): %v", model, err)
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s 响应异常: status=%d body=%s", model, resp.StatusCode, string(bodyBytes))
		}
	}

	doResponses(badModel)
	assertRecordedCalls([]string{badUpstreamModel}, []string{badUpstreamModel})

	doResponses(badModel)
	assertRecordedCalls([]string{badUpstreamModel}, []string{badUpstreamModel, badUpstreamModel})

	doResponses(goodModel)
	assertRecordedCalls([]string{badUpstreamModel, goodUpstreamModel}, []string{badUpstreamModel, badUpstreamModel})

	sessionCookie := loginAsRoot(t, ts.URL, client, rootUserID)

	getAdmin := func(path string, out any) {
		t.Helper()

		req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if err != nil {
			t.Fatalf("NewRequest(%s): %v", path, err)
		}
		req.Header.Set("Cookie", sessionCookie)
		req.Header.Set("Realms-User", strconv.FormatInt(rootUserID, 10))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do(%s): %v", path, err)
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, resp.StatusCode, string(bodyBytes))
		}
		if err := json.Unmarshal(bodyBytes, out); err != nil {
			t.Fatalf("Unmarshal(%s): %v body=%s", path, err, string(bodyBytes))
		}
	}

	var modelsResp struct {
		Success bool `json:"success"`
		Data    []struct {
			ID       int64          `json:"id"`
			PublicID string         `json:"public_id"`
			Runtime  runtimeInfoE2E `json:"runtime"`
		} `json:"data"`
	}
	getAdmin("/api/channel/"+strconv.FormatInt(primaryChannelID, 10)+"/models", &modelsResp)
	if !modelsResp.Success {
		t.Fatalf("channel models API 应返回 success=true")
	}

	var badRuntime, goodRuntime runtimeInfoE2E
	foundBad := false
	foundGood := false
	for _, item := range modelsResp.Data {
		switch item.ID {
		case primaryBadBindingID:
			foundBad = true
			badRuntime = item.Runtime
			if item.PublicID != badModel {
				t.Fatalf("bad binding public_id=%q want=%q", item.PublicID, badModel)
			}
		case primaryGoodBindingID:
			foundGood = true
			goodRuntime = item.Runtime
			if item.PublicID != goodModel {
				t.Fatalf("good binding public_id=%q want=%q", item.PublicID, goodModel)
			}
		}
	}
	if !foundBad || !foundGood {
		t.Fatalf("主渠道 bindings 不完整: foundBad=%v foundGood=%v data=%+v", foundBad, foundGood, modelsResp.Data)
	}
	if !badRuntime.Available || badRuntime.FailScore == 0 || !badRuntime.BannedActive {
		t.Fatalf("bad binding runtime 不符合预期: %+v", badRuntime)
	}
	if !goodRuntime.Available || goodRuntime.FailScore != 0 || goodRuntime.BannedActive {
		t.Fatalf("good binding runtime 不符合预期: %+v", goodRuntime)
	}

	var channelsResp struct {
		Success bool `json:"success"`
		Data    struct {
			Channels []struct {
				ID      int64          `json:"id"`
				Runtime runtimeInfoE2E `json:"runtime"`
			} `json:"channels"`
		} `json:"data"`
	}
	getAdmin("/api/channel/page", &channelsResp)
	if !channelsResp.Success {
		t.Fatalf("channel page API 应返回 success=true")
	}

	foundPrimaryChannel := false
	for _, item := range channelsResp.Data.Channels {
		if item.ID != primaryChannelID {
			continue
		}
		foundPrimaryChannel = true
		if !item.Runtime.Available || item.Runtime.FailScore != 0 || item.Runtime.BannedActive {
			t.Fatalf("primary channel runtime 不应被模型错误污染: %+v", item.Runtime)
		}
	}
	if !foundPrimaryChannel {
		t.Fatalf("未在 channels page 中找到主渠道 %d", primaryChannelID)
	}
}

func extractForwardedModelE2E(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &payload)
	return strings.TrimSpace(payload.Model)
}
