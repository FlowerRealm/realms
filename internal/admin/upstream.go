package admin

import (
	"context"
	"net/http"

	"realms/internal/scheduler"
)

type UpstreamDoer interface {
	Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)
}
