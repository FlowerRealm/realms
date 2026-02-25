//go:build embed_web_self

package realms

import "embed"

// WebSelfDistFS embeds the self-mode frontend build output.
//
// Build requirement: `web/dist-self` must exist before `go build -tags embed_web_self`.
//
//go:embed web/dist-self
var WebSelfDistFS embed.FS
