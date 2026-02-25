//go:build !embed_web_self

package realms

import "embed"

// WebSelfDistFS is empty unless built with `-tags embed_web_self`.
var WebSelfDistFS embed.FS
