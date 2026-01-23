package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"realms/internal/scheduler"
	"realms/internal/store"
)

type fakeChannelTestStore struct {
	endpointsByChannel       map[int64][]store.UpstreamEndpoint
	credsByEndpoint          map[int64][]store.OpenAICompatibleCredential
	anthropicCredsByEndpoint map[int64][]store.AnthropicCredential
	accsByEndpoint           map[int64][]store.CodexOAuthAccount
	channelModelsByChannel   map[int64][]store.ChannelModel
	enabledModels            map[string]store.ManagedModel

	updates []testUpdate
}

type testUpdate struct {
	channelID int64
	ok        bool
	latencyMS int
}

func (f *fakeChannelTestStore) ListUpstreamEndpointsByChannel(_ context.Context, channelID int64) ([]store.UpstreamEndpoint, error) {
	return f.endpointsByChannel[channelID], nil
}

func (f *fakeChannelTestStore) ListOpenAICompatibleCredentialsByEndpoint(_ context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error) {
	return f.credsByEndpoint[endpointID], nil
}

func (f *fakeChannelTestStore) ListAnthropicCredentialsByEndpoint(_ context.Context, endpointID int64) ([]store.AnthropicCredential, error) {
	if f.anthropicCredsByEndpoint == nil {
		return nil, nil
	}
	return f.anthropicCredsByEndpoint[endpointID], nil
}

func (f *fakeChannelTestStore) ListCodexOAuthAccountsByEndpoint(_ context.Context, endpointID int64) ([]store.CodexOAuthAccount, error) {
	return f.accsByEndpoint[endpointID], nil
}

func (f *fakeChannelTestStore) ListChannelModelsByChannelID(_ context.Context, channelID int64) ([]store.ChannelModel, error) {
	return f.channelModelsByChannel[channelID], nil
}

func (f *fakeChannelTestStore) GetEnabledManagedModelByPublicID(_ context.Context, publicID string) (store.ManagedModel, error) {
	m, ok := f.enabledModels[publicID]
	if !ok || m.Status != 1 {
		return store.ManagedModel{}, errors.New("not found")
	}
	return m, nil
}

func (f *fakeChannelTestStore) UpdateUpstreamChannelTest(_ context.Context, channelID int64, ok bool, latencyMS int) error {
	f.updates = append(f.updates, testUpdate{channelID: channelID, ok: ok, latencyMS: latencyMS})
	return nil
}

type fakeUpstreamDoer struct {
	gotSel    scheduler.Selection
	gotSels   []scheduler.Selection
	gotPath   string
	gotBodies []map[string]any

	respByModel map[string]struct {
		status int
		body   string
		header http.Header
		err    error
	}

	respByCredential map[int64]struct {
		status int
		body   string
		header http.Header
		err    error
	}
}

func (d *fakeUpstreamDoer) Do(_ context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
	d.gotSel = sel
	d.gotSels = append(d.gotSels, sel)
	if downstream != nil && downstream.URL != nil {
		d.gotPath = downstream.URL.Path
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	d.gotBodies = append(d.gotBodies, got)

	model, _ := got["model"].(string)
	if d.respByCredential != nil {
		if resp, ok := d.respByCredential[sel.CredentialID]; ok {
			if resp.err != nil {
				return nil, resp.err
			}
			h := make(http.Header)
			for k, vs := range resp.header {
				for _, v := range vs {
					h.Add(k, v)
				}
			}
			return &http.Response{
				StatusCode: resp.status,
				Body:       io.NopCloser(strings.NewReader(resp.body)),
				Header:     h,
			}, nil
		}
	}
	if d.respByModel != nil {
		if resp, ok := d.respByModel[model]; ok {
			if resp.err != nil {
				return nil, resp.err
			}
			h := make(http.Header)
			for k, vs := range resp.header {
				for _, v := range vs {
					h.Add(k, v)
				}
			}
			return &http.Response{
				StatusCode: resp.status,
				Body:       io.NopCloser(strings.NewReader(resp.body)),
				Header:     h,
			}, nil
		}
	}
	return nil, errors.New("unexpected model")
}

func TestRunChannelTest_OpenAI_OK(t *testing.T) {
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			1: {store.UpstreamEndpoint{ID: 10, ChannelID: 1, BaseURL: "https://example.com", Status: 1, Priority: 0}},
		},
		credsByEndpoint: map[int64][]store.OpenAICompatibleCredential{
			10: {store.OpenAICompatibleCredential{ID: 100, EndpointID: 10, Status: 1}},
		},
		channelModelsByChannel: map[int64][]store.ChannelModel{
			1: {store.ChannelModel{ID: 1, ChannelID: 1, PublicID: "m1", UpstreamModel: "m1-up", Status: 1}},
		},
		enabledModels: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
	}
	doer := &fakeUpstreamDoer{
		respByModel: map[string]struct {
			status int
			body   string
			header http.Header
			err    error
		}{
			"m1-up": {status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n"},
		},
	}

	msg, err := runChannelTest(context.Background(), st, doer, store.UpstreamChannel{ID: 1, Type: store.UpstreamTypeOpenAICompatible}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected non-empty message")
	}
	if doer.gotSel.CredentialType != scheduler.CredentialTypeOpenAI || doer.gotSel.CredentialID != 100 {
		t.Fatalf("unexpected selection: %+v", doer.gotSel)
	}
	if doer.gotPath != "/v1/responses" {
		t.Fatalf("unexpected path: %q", doer.gotPath)
	}
	if len(doer.gotBodies) != 1 {
		t.Fatalf("expected 1 call, got %d", len(doer.gotBodies))
	}
	inputArr, ok := doer.gotBodies[0]["input"].([]any)
	if !ok || len(inputArr) != 1 {
		t.Fatalf("unexpected input: %#v", doer.gotBodies[0]["input"])
	}
	inputMsg, ok := inputArr[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected input message: %#v", inputArr[0])
	}
	if inputMsg["role"] != "user" || inputMsg["content"] != defaultTestInput() {
		t.Fatalf("unexpected input message: %#v", inputMsg)
	}
	if doer.gotBodies[0]["model"] != "m1-up" {
		t.Fatalf("unexpected model: %#v", doer.gotBodies[0]["model"])
	}
	if doer.gotBodies[0]["stream"] != true {
		t.Fatalf("expected stream=true, got %#v", doer.gotBodies[0]["stream"])
	}
	if _, ok := doer.gotBodies[0]["max_output_tokens"]; !ok {
		t.Fatalf("missing max_output_tokens")
	}
	if len(st.updates) != 1 || st.updates[0].channelID != 1 || !st.updates[0].ok || st.updates[0].latencyMS < 0 {
		t.Fatalf("unexpected updates: %+v", st.updates)
	}
}

func TestRunChannelTest_CodexOAuth_FailoverNextAccount(t *testing.T) {
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			2: {store.UpstreamEndpoint{ID: 20, ChannelID: 2, BaseURL: "https://chatgpt.com/backend-api/codex", Status: 1, Priority: 0}},
		},
		accsByEndpoint: map[int64][]store.CodexOAuthAccount{
			20: {
				store.CodexOAuthAccount{ID: 201, EndpointID: 20, Status: 1},
				store.CodexOAuthAccount{ID: 200, EndpointID: 20, Status: 1},
			},
		},
	}
	doer := &fakeUpstreamDoer{
		respByCredential: map[int64]struct {
			status int
			body   string
			header http.Header
			err    error
		}{
			201: {status: http.StatusUnauthorized, header: http.Header{"Content-Type": []string{"application/json"}}, body: "{\"error\":{\"message\":\"invalid token\"}}"},
			200: {status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n"},
		},
	}

	msg, err := runChannelTest(context.Background(), st, doer, store.UpstreamChannel{ID: 2, Type: store.UpstreamTypeCodexOAuth}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected non-empty message")
	}
	if len(doer.gotSels) != 2 {
		t.Fatalf("expected 2 calls (failover), got %d", len(doer.gotSels))
	}
	if doer.gotSels[0].CredentialType != scheduler.CredentialTypeCodex || doer.gotSels[0].CredentialID != 201 {
		t.Fatalf("unexpected first selection: %+v", doer.gotSels[0])
	}
	if doer.gotSels[1].CredentialType != scheduler.CredentialTypeCodex || doer.gotSels[1].CredentialID != 200 {
		t.Fatalf("unexpected second selection: %+v", doer.gotSels[1])
	}
	if len(doer.gotBodies) != 2 {
		t.Fatalf("expected 2 bodies (failover), got %d", len(doer.gotBodies))
	}
	for i, body := range doer.gotBodies {
		instructions, _ := body["instructions"].(string)
		if strings.TrimSpace(instructions) == "" {
			t.Fatalf("expected non-empty instructions in body[%d]", i)
		}
		if _, ok := body["input"].([]any); !ok {
			t.Fatalf("expected input to be array in body[%d], got %#v", i, body["input"])
		}
		if v, ok := body["store"].(bool); !ok || v {
			t.Fatalf("expected store=false in body[%d], got %#v", i, body["store"])
		}
		if v, ok := body["parallel_tool_calls"].(bool); !ok || !v {
			t.Fatalf("expected parallel_tool_calls=true in body[%d], got %#v", i, body["parallel_tool_calls"])
		}
		if _, ok := body["include"].([]any); !ok {
			t.Fatalf("expected include to be array in body[%d], got %#v", i, body["include"])
		}
	}
	if len(st.updates) != 1 || st.updates[0].channelID != 2 || !st.updates[0].ok || st.updates[0].latencyMS < 0 {
		t.Fatalf("unexpected updates: %+v", st.updates)
	}
}

func TestRunChannelTest_OpenAI_SSEWithoutContentTypeStillOK(t *testing.T) {
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			1: {store.UpstreamEndpoint{ID: 10, ChannelID: 1, BaseURL: "https://example.com", Status: 1, Priority: 0}},
		},
		credsByEndpoint: map[int64][]store.OpenAICompatibleCredential{
			10: {store.OpenAICompatibleCredential{ID: 100, EndpointID: 10, Status: 1}},
		},
		channelModelsByChannel: map[int64][]store.ChannelModel{
			1: {store.ChannelModel{ID: 1, ChannelID: 1, PublicID: "m1", UpstreamModel: "m1-up", Status: 1}},
		},
		enabledModels: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
	}
	doer := &fakeUpstreamDoer{
		respByModel: map[string]struct {
			status int
			body   string
			header http.Header
			err    error
		}{
			"m1-up": {
				status: http.StatusOK,
				header: http.Header{},
				body:   "event: response.created\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n",
			},
		},
	}

	msg, err := runChannelTest(context.Background(), st, doer, store.UpstreamChannel{ID: 1, Type: store.UpstreamTypeOpenAICompatible}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected non-empty message")
	}
	if len(st.updates) != 1 || st.updates[0].channelID != 1 || !st.updates[0].ok {
		t.Fatalf("unexpected updates: %+v", st.updates)
	}
}

func TestRunChannelTest_NoAvailableCredential(t *testing.T) {
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			1: {store.UpstreamEndpoint{ID: 10, ChannelID: 1, BaseURL: "https://example.com", Status: 1, Priority: 0}},
		},
		credsByEndpoint: map[int64][]store.OpenAICompatibleCredential{
			10: {store.OpenAICompatibleCredential{ID: 100, EndpointID: 10, Status: 0}},
		},
		channelModelsByChannel: map[int64][]store.ChannelModel{
			1: {store.ChannelModel{ID: 1, ChannelID: 1, PublicID: "m1", UpstreamModel: "m1-up", Status: 1}},
		},
		enabledModels: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
	}
	doer := &fakeUpstreamDoer{respByModel: map[string]struct {
		status int
		body   string
		header http.Header
		err    error
	}{}}

	_, err := runChannelTest(context.Background(), st, doer, store.UpstreamChannel{ID: 1, Type: store.UpstreamTypeOpenAICompatible}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "测试准备失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.updates) != 1 || st.updates[0].channelID != 1 || st.updates[0].ok || st.updates[0].latencyMS != 0 {
		t.Fatalf("unexpected updates: %+v", st.updates)
	}
}

func TestRunChannelTest_UpstreamError(t *testing.T) {
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			1: {store.UpstreamEndpoint{ID: 10, ChannelID: 1, BaseURL: "https://example.com", Status: 1, Priority: 0}},
		},
		credsByEndpoint: map[int64][]store.OpenAICompatibleCredential{
			10: {store.OpenAICompatibleCredential{ID: 100, EndpointID: 10, Status: 1}},
		},
		channelModelsByChannel: map[int64][]store.ChannelModel{
			1: {store.ChannelModel{ID: 1, ChannelID: 1, PublicID: "m1", UpstreamModel: "m1-up", Status: 1}},
		},
		enabledModels: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
	}
	doer := &fakeUpstreamDoer{
		respByModel: map[string]struct {
			status int
			body   string
			header http.Header
			err    error
		}{
			"m1-up": {err: errors.New("dial tcp")},
		},
	}

	_, err := runChannelTest(context.Background(), st, doer, store.UpstreamChannel{ID: 1, Type: store.UpstreamTypeOpenAICompatible}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(st.updates) != 1 || st.updates[0].channelID != 1 || st.updates[0].ok {
		t.Fatalf("unexpected updates: %+v", st.updates)
	}
	// latency 可能为 0（测试环境很快），只要非负即可
	if st.updates[0].latencyMS < 0 {
		t.Fatalf("unexpected latency: %d", st.updates[0].latencyMS)
	}
}

func TestSelectChannelTestSelection_CodexCooldown(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			2: {store.UpstreamEndpoint{ID: 20, ChannelID: 2, BaseURL: "https://chatgpt.com/backend-api/codex", Status: 1, Priority: 0}},
		},
		accsByEndpoint: map[int64][]store.CodexOAuthAccount{
			20: {store.CodexOAuthAccount{ID: 200, EndpointID: 20, Status: 1, CooldownUntil: &future}},
		},
	}

	_, err := selectChannelTestSelection(context.Background(), st, store.UpstreamChannel{ID: 2, Type: store.UpstreamTypeCodexOAuth}, time.Now())
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunChannelTest_MultiModels_PartialFailureFails(t *testing.T) {
	st := &fakeChannelTestStore{
		endpointsByChannel: map[int64][]store.UpstreamEndpoint{
			1: {store.UpstreamEndpoint{ID: 10, ChannelID: 1, BaseURL: "https://example.com", Status: 1, Priority: 0}},
		},
		credsByEndpoint: map[int64][]store.OpenAICompatibleCredential{
			10: {store.OpenAICompatibleCredential{ID: 100, EndpointID: 10, Status: 1}},
		},
		channelModelsByChannel: map[int64][]store.ChannelModel{
			1: {
				store.ChannelModel{ID: 1, ChannelID: 1, PublicID: "m1", UpstreamModel: "ok", Status: 1},
				store.ChannelModel{ID: 2, ChannelID: 1, PublicID: "m2", UpstreamModel: "bad", Status: 1},
			},
		},
		enabledModels: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
			"m2": {ID: 2, PublicID: "m2", Status: 1},
		},
	}
	doer := &fakeUpstreamDoer{
		respByModel: map[string]struct {
			status int
			body   string
			header http.Header
			err    error
		}{
			"ok":  {status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n"},
			"bad": {status: http.StatusBadRequest, header: http.Header{"Content-Type": []string{"application/json"}}, body: "{\"error\":{\"message\":\"model not found\"}}"},
		},
	}

	_, err := runChannelTest(context.Background(), st, doer, store.UpstreamChannel{ID: 1, Type: store.UpstreamTypeOpenAICompatible}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(st.updates) != 1 || st.updates[0].channelID != 1 || st.updates[0].ok {
		t.Fatalf("unexpected updates: %+v", st.updates)
	}
	if len(doer.gotBodies) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(doer.gotBodies))
	}
}
