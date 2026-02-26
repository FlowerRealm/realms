//go:build embed_web_personal

package realms

import "embed"

// WebPersonalDistFS embeds the personal-mode frontend build output.
//
// Build requirement: `web/dist-personal` must exist before `go build -tags embed_web_personal`.
//
//go:embed web/dist-personal
var WebPersonalDistFS embed.FS
