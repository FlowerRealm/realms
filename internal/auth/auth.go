// Package auth 提供统一的主体信息（token/session）与密码/随机数工具，便于鉴权与审计。
package auth

import (
	"context"
)

type ActorType string

const (
	ActorTypeToken   ActorType = "token"
	ActorTypeSession ActorType = "session"
)

type Principal struct {
	ActorType ActorType
	UserID    int64
	TokenID   *int64
	Role      string
	// Groups 用于数据面“渠道组路由/模型 ACL”的有序渠道组集合（通常来自 token_channel_groups，并受 users.main_group 限制）。
	Groups    []string
	CSRFToken *string
}

type ctxKey int

const principalKey ctxKey = 1

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	v := ctx.Value(principalKey)
	if v == nil {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}
