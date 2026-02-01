//go:build !embed_web

package realms

import "embed"

// WebDistFS is empty unless built with `-tags embed_web`.
var WebDistFS embed.FS
