package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

func TestAdminChannelGroups_List_IncludesPointerEvenWhenNotPinned(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	engine, sessionCookie, userID := setupRootSession(t, st)

	ctx := context.Background()
	groupID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, groupID, channelID, 0, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel: %v", err)
	}
	if err := st.UpsertChannelGroupPointer(ctx, store.ChannelGroupPointer{
		GroupID:   groupID,
		ChannelID: channelID,
		Pinned:    false,
		Reason:    "route",
	}); err != nil {
		t.Fatalf("UpsertChannelGroupPointer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/channel-groups", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list channel-groups status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID               int64  `json:"id"`
			PointerChannelID int64  `json:"pointer_channel_id"`
			PointerChannel   string `json:"pointer_channel_name"`
			PointerPinned    bool   `json:"pointer_pinned"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got message=%q", resp.Message)
	}

	found := false
	for _, g := range resp.Data {
		if g.ID != groupID {
			continue
		}
		found = true
		if g.PointerChannelID != channelID {
			t.Fatalf("expected pointer_channel_id=%d, got=%d", channelID, g.PointerChannelID)
		}
		if g.PointerChannel != "c1" {
			t.Fatalf("expected pointer_channel_name=c1, got=%q", g.PointerChannel)
		}
		if g.PointerPinned {
			t.Fatalf("expected pointer_pinned=false")
		}
	}
	if !found {
		t.Fatalf("expected group %d to exist in response", groupID)
	}
}

func TestAdminChannelGroups_List_DefaultPointerWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	engine, sessionCookie, userID := setupRootSession(t, st)

	ctx := context.Background()
	groupID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	c1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel c1: %v", err)
	}
	c2, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c2", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel c2: %v", err)
	}

	if err := st.AddChannelGroupMemberChannel(ctx, groupID, c1, 10, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel c1: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, groupID, c2, 20, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel c2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/channel-groups", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list channel-groups status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID               int64  `json:"id"`
			PointerChannelID int64  `json:"pointer_channel_id"`
			PointerChannel   string `json:"pointer_channel_name"`
			PointerPinned    bool   `json:"pointer_pinned"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got message=%q", resp.Message)
	}

	found := false
	for _, g := range resp.Data {
		if g.ID != groupID {
			continue
		}
		found = true
		if g.PointerChannelID != c2 {
			t.Fatalf("expected default pointer_channel_id=%d, got=%d", c2, g.PointerChannelID)
		}
		if g.PointerChannel != "c2" {
			t.Fatalf("expected default pointer_channel_name=c2, got=%q", g.PointerChannel)
		}
		if g.PointerPinned {
			t.Fatalf("expected default pointer_pinned=false")
		}
	}
	if !found {
		t.Fatalf("expected group %d to exist in response", groupID)
	}
}

func TestAdminChannelGroupPointerCandidates_RecursiveIncludesNestedChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	engine, sessionCookie, userID := setupRootSession(t, st)

	ctx := context.Background()
	parentID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup parent: %v", err)
	}
	childID, err := st.CreateChannelGroup(ctx, "g1_child", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup child: %v", err)
	}
	if err := st.AddChannelGroupMemberGroup(ctx, parentID, childID, 0, false); err != nil {
		t.Fatalf("AddChannelGroupMemberGroup: %v", err)
	}
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, childID, channelID, 0, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://example.com/api/admin/channel-groups/%d/pointer/candidates", parentID), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list pointer candidates status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got message=%q", resp.Message)
	}
	found := false
	for _, c := range resp.Data {
		if c.ID == channelID {
			found = true
			if c.Name != "c1" {
				t.Fatalf("expected candidate name=c1, got %q", c.Name)
			}
		}
	}
	if !found {
		t.Fatalf("expected channel %d to be present in candidates", channelID)
	}
}
