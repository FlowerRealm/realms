package openai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"realms/internal/auth"
	"realms/internal/scheduler"
	"realms/internal/store"
)

const (
	openAIObjectTypeResponse       = "response"
	openAIObjectTypeChatCompletion = "chat_completion"

	realmsOwnerMetadataKey = "realms_owner"
)

func realmsOwnerTagForUser(userID int64) string {
	if userID <= 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("realms:%d", userID)))
	return hex.EncodeToString(sum[:])
}

func chatCompletionRequestStoresObject(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	v := gjson.GetBytes(body, "store")
	switch v.Type {
	case gjson.True:
		return true
	case gjson.String:
		s := strings.TrimSpace(v.String())
		return s == "1" || strings.EqualFold(s, "true")
	default:
		return false
	}
}

func isLikelyResponseID(id string) bool {
	return strings.HasPrefix(id, "resp_")
}

func isLikelyChatCompletionID(id string) bool {
	return strings.HasPrefix(id, "chatcmpl-") || strings.HasPrefix(id, "chatcmpl_") || strings.HasPrefix(id, "cmpl-")
}

func extractResponseIDFromJSONBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if id := strings.TrimSpace(gjson.GetBytes(b, "id").String()); isLikelyResponseID(id) {
		return id
	}
	if id := strings.TrimSpace(gjson.GetBytes(b, "response.id").String()); isLikelyResponseID(id) {
		return id
	}
	if id := strings.TrimSpace(gjson.GetBytes(b, "response_id").String()); isLikelyResponseID(id) {
		return id
	}
	return ""
}

func extractChatCompletionIDFromJSONBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if id := strings.TrimSpace(gjson.GetBytes(b, "id").String()); isLikelyChatCompletionID(id) {
		return id
	}
	return ""
}

func (h *Handler) recordOpenAIObjectRef(ctx context.Context, objectType string, objectID string, p auth.Principal, sel scheduler.Selection) {
	if h == nil || h.refs == nil {
		return
	}
	if p.UserID <= 0 {
		return
	}
	objectType = strings.TrimSpace(objectType)
	objectID = strings.TrimSpace(objectID)
	if objectType == "" || objectID == "" {
		return
	}
	tokenID := int64(0)
	if p.TokenID != nil {
		tokenID = *p.TokenID
	}
	rawSel, err := json.Marshal(sel)
	if err != nil || len(rawSel) == 0 {
		return
	}
	_ = h.refs.UpsertOpenAIObjectRef(ctx, store.OpenAIObjectRef{
		ObjectType:    objectType,
		ObjectID:      objectID,
		UserID:        p.UserID,
		TokenID:       tokenID,
		SelectionJSON: string(rawSel),
	})
	h.touchBindingFromRouteKey(p.UserID, sel, objectID)
}

func (h *Handler) ownedSelection(ctx context.Context, p auth.Principal, objectType string, objectID string) (scheduler.Selection, bool) {
	if h == nil || h.refs == nil {
		return scheduler.Selection{}, false
	}
	if p.UserID <= 0 {
		return scheduler.Selection{}, false
	}
	ref, ok, err := h.refs.GetOpenAIObjectRefForUser(ctx, p.UserID, objectType, objectID)
	if err != nil || !ok {
		return scheduler.Selection{}, false
	}
	var sel scheduler.Selection
	if err := json.Unmarshal([]byte(ref.SelectionJSON), &sel); err != nil {
		return scheduler.Selection{}, false
	}
	h.touchBindingFromRouteKey(p.UserID, sel, objectID)
	return sel, true
}

func writeNotFound(w http.ResponseWriter) {
	if w == nil {
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}
