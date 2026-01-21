package codexoauth

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	ErrStateGenerationFailed ErrorCode = "state_generation_failed"
	ErrPKCEGenerationFailed  ErrorCode = "pkce_generation_failed"
	ErrAuthorizeURLFailed    ErrorCode = "authorize_url_failed"
	ErrCodeExchangeFailed    ErrorCode = "code_exchange_failed"
	ErrRefreshFailed         ErrorCode = "refresh_failed"

	ErrCallbackUpstreamError         ErrorCode = "callback_upstream_error"
	ErrCallbackInvalidParams         ErrorCode = "callback_invalid_params"
	ErrCallbackInvalidOrExpiredState ErrorCode = "callback_invalid_or_expired_state"
	ErrCallbackExpiredState          ErrorCode = "callback_expired_state"
	ErrCallbackMissingSessionCookie  ErrorCode = "callback_missing_session_cookie"
	ErrCallbackInvalidSession        ErrorCode = "callback_invalid_session"
	ErrCallbackActorMismatch         ErrorCode = "callback_actor_mismatch"
	ErrCallbackUserNotFound          ErrorCode = "callback_user_not_found"
	ErrCallbackForbidden             ErrorCode = "callback_forbidden"

	ErrIDTokenParseFailed ErrorCode = "id_token_parse_failed"
	ErrMissingAccountID   ErrorCode = "missing_account_id"
	ErrStoreFailed        ErrorCode = "store_failed"

	ErrCallbackServerPortInUse   ErrorCode = "callback_server_port_in_use"
	ErrCallbackServerStartFailed ErrorCode = "callback_server_start_failed"
)

type Error struct {
	Code  ErrorCode
	Cause error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("codexoauth: %s", e.Code)
	}
	return fmt.Sprintf("codexoauth: %s: %v", e.Code, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func Wrap(code ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Cause: err}
}

func CodeOf(err error) (ErrorCode, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Code, true
	}
	return "", false
}

func UserMessage(err error) string {
	code, ok := CodeOf(err)
	if !ok {
		return "发生未知错误"
	}
	switch code {
	case ErrStateGenerationFailed, ErrPKCEGenerationFailed, ErrAuthorizeURLFailed:
		return "发起 OAuth 失败，请重试"
	case ErrCallbackUpstreamError:
		return "OAuth 回调失败：上游返回错误"
	case ErrCallbackInvalidParams:
		return "OAuth 回调失败：参数错误"
	case ErrCallbackInvalidOrExpiredState, ErrCallbackExpiredState:
		return "OAuth 回调失败：state 无效或已过期，请重新发起授权"
	case ErrCallbackMissingSessionCookie:
		return "OAuth 回调失败：未登录或会话已过期"
	case ErrCallbackInvalidSession:
		return "OAuth 回调失败：会话无效，请重新登录"
	case ErrCallbackActorMismatch:
		return "OAuth 回调失败：会话与发起人不匹配"
	case ErrCallbackUserNotFound:
		return "OAuth 回调失败：用户不存在"
	case ErrCallbackForbidden:
		return "OAuth 回调失败：无权限"
	case ErrCodeExchangeFailed:
		var te *TokenEndpointError
		if errors.As(err, &te) && te != nil {
			if te.ErrorCode == "invalid_grant" {
				return "OAuth 回调失败：授权码无效或已过期，请重新发起授权"
			}
			if te.StatusCode == 429 || te.StatusCode >= 500 {
				return "OAuth 回调失败：换取 token 失败（上游暂时不可用），请稍后重试"
			}
			var detail strings.Builder
			if te.StatusCode != 0 {
				detail.WriteString(fmt.Sprintf("%d", te.StatusCode))
			}
			if strings.TrimSpace(te.ErrorCode) != "" {
				if detail.Len() > 0 {
					detail.WriteString(" ")
				}
				detail.WriteString(strings.TrimSpace(te.ErrorCode))
			}
			if strings.TrimSpace(te.ErrorDescription) != "" {
				detail.WriteString(": ")
				detail.WriteString(strings.TrimSpace(te.ErrorDescription))
			} else if strings.TrimSpace(te.BodySnippet) != "" {
				detail.WriteString(": ")
				detail.WriteString(strings.TrimSpace(te.BodySnippet))
			}
			if detail.Len() > 0 {
				return "OAuth 回调失败：换取 token 失败（" + detail.String() + "）"
			}
		}
		if cause := errors.Unwrap(err); cause != nil {
			msg := strings.TrimSpace(cause.Error())
			if msg != "" {
				if strings.Contains(msg, "TLS handshake timeout") || strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "context deadline exceeded") {
					return "OAuth 回调失败：换取 token 失败（" + msg + "；请检查运行 Realms 的机器网络/DNS/代理，确保可访问 auth.openai.com）"
				}
				return "OAuth 回调失败：换取 token 失败（" + msg + "）"
			}
		}
		return "OAuth 回调失败：换取 token 失败，请稍后重试"
	case ErrRefreshFailed:
		return "刷新 token 失败，请稍后重试"
	case ErrIDTokenParseFailed, ErrMissingAccountID:
		return "OAuth 回调失败：解析账号信息失败，请重试"
	case ErrStoreFailed:
		return "OAuth 回调失败：保存失败，请重试"
	case ErrCallbackServerPortInUse:
		return "Codex OAuth 回调监听端口被占用，请检查配置或释放端口"
	case ErrCallbackServerStartFailed:
		return "Codex OAuth 回调监听启动失败"
	default:
		return "发生未知错误"
	}
}
