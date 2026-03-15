package router

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

func createLoggedInUser(t *testing.T, st *store.Store, engine *gin.Engine, cookieName, email, username, password string) (int64, string) {
	t.Helper()
	ctx := context.Background()
	pwHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, email, username, pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	return userID, loginCookie(t, engine, cookieName, email, password)
}

func ensureDefaultMainGroupForTest(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreateMainGroup(ctx, "default", nil, 1); err != nil && !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("CreateMainGroup(default): %v", err)
	}
}

func TestRedeemCodeRoute_BalanceSuccess(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, cookieName := newTestEngine(t, st)

	userID, sessionCookie := createLoggedInUser(t, st, engine, cookieName, "redeem-route@example.com", "redeemroute", "password123")
	ctx := context.Background()
	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "route-balance",
		Code:             "ROUTE-BAL",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("3"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"code": "route-bal"})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/billing/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			RewardType    string `json:"reward_type"`
			BalanceUSD    string `json:"balance_usd"`
			NewBalanceUSD string `json:"new_balance_usd"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got message=%q", resp.Message)
	}
	if resp.Data.RewardType != "balance" || resp.Data.NewBalanceUSD != "3" {
		t.Fatalf("unexpected response data: %+v", resp.Data)
	}
}

func TestRedeemCodeRoute_SamePlanNeedsMode(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, cookieName := newTestEngine(t, st)

	userID, sessionCookie := createLoggedInUser(t, st, engine, cookieName, "redeem-mode@example.com", "redeemmode", "password123")
	ctx := context.Background()
	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "route_plan",
		Name:         "Route Plan",
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	if _, _, err := st.PurchaseSubscriptionByPlanID(ctx, userID, planID, time.Now()); err != nil {
		t.Fatalf("PurchaseSubscriptionByPlanID: %v", err)
	}
	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:          "route-same-plan",
		Code:               "ROUTE-SAME",
		DistributionMode:   store.RedemptionCodeDistributionSingle,
		RewardType:         store.RedemptionCodeRewardSubscription,
		SubscriptionPlanID: &planID,
		MaxRedemptions:     1,
		Status:             store.RedemptionCodeStatusActive,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"code": "ROUTE-SAME"})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/billing/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Success || resp.Data.ErrorCode != "subscription_activation_mode_required" {
		t.Fatalf("expected mode required response, got body=%s", rr.Body.String())
	}
}

func TestRedeemCodeRoute_InvalidSubscriptionActivationModeRejected(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, cookieName := newTestEngine(t, st)

	userID, sessionCookie := createLoggedInUser(t, st, engine, cookieName, "redeem-invalid-mode@example.com", "redeeminvalidmode", "password123")
	ctx := context.Background()
	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "route_invalid_mode_plan",
		Name:         "Route Invalid Mode Plan",
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:          "route-invalid-mode",
		Code:               "ROUTE-INVALID-MODE",
		DistributionMode:   store.RedemptionCodeDistributionSingle,
		RewardType:         store.RedemptionCodeRewardSubscription,
		SubscriptionPlanID: &planID,
		MaxRedemptions:     1,
		Status:             store.RedemptionCodeStatusActive,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"kind":                         "subscription",
		"code":                         "ROUTE-INVALID-MODE",
		"subscription_activation_mode": "weird",
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/billing/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Success || resp.Message != "subscription_activation_mode 不合法" {
		t.Fatalf("expected invalid mode response, got body=%s", rr.Body.String())
	}
}

func TestAdminRedemptionCodesExportRoute(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	ctx := context.Background()
	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "export-batch",
		Code:             "EXPORT-1",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("1.5"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
		CreatedBy:        rootID,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/redemption-codes/export", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	reader := csv.NewReader(strings.NewReader(rr.Body.String()))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll CSV: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 csv rows, got %d", len(rows))
	}
	if rows[1][0] != "EXPORT-1" {
		t.Fatalf("unexpected first data row: %v", rows[1])
	}
}

func TestRedeemCodeRoute_RewardTypeMismatch(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, cookieName := newTestEngine(t, st)

	userID, sessionCookie := createLoggedInUser(t, st, engine, cookieName, "redeem-mismatch@example.com", "redeemmismatch", "password123")
	ctx := context.Background()
	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "route-mismatch",
		Code:             "MISMATCH-BAL",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("3"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"kind": "subscription", "code": "MISMATCH-BAL"})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/billing/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Success || resp.Message != "兑换码类型不匹配" {
		t.Fatalf("expected reward mismatch response, got body=%s", rr.Body.String())
	}
}

func TestRedeemCodeRoute_InvalidKindRejected(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, cookieName := newTestEngine(t, st)

	userID, sessionCookie := createLoggedInUser(t, st, engine, cookieName, "redeem-invalid-kind@example.com", "redeeminvalidkind", "password123")

	body, _ := json.Marshal(map[string]any{"kind": "weird", "code": "ANY-CODE"})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/billing/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Success || resp.Message != "kind 不合法" {
		t.Fatalf("expected invalid kind response, got body=%s", rr.Body.String())
	}
}

func TestAdminCreateRedemptionCodesRoute_GenerateCodes(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	ctx := context.Background()
	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "plan_generated",
		Name:         "Generated Plan",
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"batch_name":           "generated-batch",
		"count":                2,
		"distribution_mode":    "single",
		"reward_type":          "subscription",
		"subscription_plan_id": planID,
		"status":               1,
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/admin/redemption-codes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			IDs   []int64  `json:"ids"`
			Codes []string `json:"codes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got body=%s", rr.Body.String())
	}
	if len(resp.Data.IDs) != 2 || len(resp.Data.Codes) != 2 {
		t.Fatalf("expected 2 generated codes, got %+v", resp.Data)
	}
	for _, code := range resp.Data.Codes {
		if strings.TrimSpace(code) == "" {
			t.Fatalf("expected non-empty generated code")
		}
	}
}

func TestAdminCreateRedemptionCodesRoute_InvalidStatusRejected(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	body, _ := json.Marshal(map[string]any{
		"batch_name":        "invalid-status",
		"distribution_mode": "single",
		"reward_type":       "balance",
		"balance_usd":       "1",
		"status":            9,
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/admin/redemption-codes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Success || resp.Message != "status 不合法" {
		t.Fatalf("expected invalid status response, got body=%s", rr.Body.String())
	}
}

func TestAdminUpdateRedemptionCodeRoute_AcceptsDatetimeLocal(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	ctx := context.Background()
	codeID, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "shared-edit",
		Code:             "SHARED-EDIT",
		DistributionMode: store.RedemptionCodeDistributionShared,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("1"),
		MaxRedemptions:   3,
		Status:           store.RedemptionCodeStatusActive,
		CreatedBy:        rootID,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"max_redemptions": 5,
		"expires_at":      "2026-03-15T12:34",
		"status":          1,
	})
	req := httptest.NewRequest(http.MethodPatch, "http://example.com/api/admin/redemption-codes/"+strconv.FormatInt(codeID, 10), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got body=%s", rr.Body.String())
	}
}

func TestAdminUpdateRedemptionCodeRoute_AllowsPartialStatusOnly(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	ctx := context.Background()
	codeID, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "shared-status-only",
		Code:             "SHARED-STATUS",
		DistributionMode: store.RedemptionCodeDistributionShared,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("2"),
		MaxRedemptions:   3,
		Status:           store.RedemptionCodeStatusActive,
		CreatedBy:        rootID,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"status": 0,
	})
	req := httptest.NewRequest(http.MethodPatch, "http://example.com/api/admin/redemption-codes/"+strconv.FormatInt(codeID, 10), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got body=%s", rr.Body.String())
	}

	item, err := st.GetRedemptionCodeByID(ctx, codeID)
	if err != nil {
		t.Fatalf("GetRedemptionCodeByID: %v", err)
	}
	if item.Code.MaxRedemptions != 3 {
		t.Fatalf("expected max_redemptions to stay 3, got %d", item.Code.MaxRedemptions)
	}
	if item.Code.Status != store.RedemptionCodeStatusDisabled {
		t.Fatalf("expected status disabled, got %d", item.Code.Status)
	}
}

func TestAdminUpdateRedemptionCodeRoute_DateOnlyExpiryUsesEndOfDay(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	ctx := context.Background()
	codeID, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "shared-date-only",
		Code:             "SHARED-DATE-ONLY",
		DistributionMode: store.RedemptionCodeDistributionShared,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("1"),
		MaxRedemptions:   2,
		Status:           store.RedemptionCodeStatusActive,
		CreatedBy:        rootID,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"expires_at": "2026-03-15",
	})
	req := httptest.NewRequest(http.MethodPatch, "http://example.com/api/admin/redemption-codes/"+strconv.FormatInt(codeID, 10), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	item, err := st.GetRedemptionCodeByID(ctx, codeID)
	if err != nil {
		t.Fatalf("GetRedemptionCodeByID: %v", err)
	}
	if item.Code.ExpiresAt == nil {
		t.Fatalf("expected expires_at to be set")
	}
	if item.Code.ExpiresAt.Hour() != 23 || item.Code.ExpiresAt.Minute() != 59 || item.Code.ExpiresAt.Second() != 59 {
		t.Fatalf("expected end-of-day expiry, got %v", item.Code.ExpiresAt)
	}
}

func TestAdminRedemptionCodesExportRoute_FiltersExhausted(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ensureDefaultMainGroupForTest(t, st)
	engine, sessionCookie, rootID := setupRootSession(t, st)

	ctx := context.Background()
	userID, err := st.CreateUser(ctx, "export-filter-user@example.com", "exportfilteruser", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}

	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "export-filter",
		Code:             "EXHAUSTED-ONLY",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("1"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
		CreatedBy:        rootID,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode exhausted: %v", err)
	}
	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "export-filter",
		Code:             "UNUSED-ONLY",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("1"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
		CreatedBy:        rootID,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode unused: %v", err)
	}
	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: userID, Code: "EXHAUSTED-ONLY", Now: time.Now()}); err != nil {
		t.Fatalf("RedeemCode exhausted: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/redemption-codes/export?exhausted=true", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	reader := csv.NewReader(strings.NewReader(rr.Body.String()))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 exhausted row, got %d rows: %v", len(rows), rows)
	}
	if rows[1][0] != "EXHAUSTED-ONLY" {
		t.Fatalf("expected exhausted row only, got %v", rows[1])
	}
}
