//go:build embed_web

package realms

import "embed"

// WebDistFS embeds the production frontend build output.
//
// Build requirement: `web/dist` must exist before `go build -tags embed_web`.
//
//go:embed web/dist
var WebDistFS embed.FS

