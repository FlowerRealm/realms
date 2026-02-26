//go:build !embed_web_personal

package realms

import "embed"

// WebPersonalDistFS is empty unless built with `-tags embed_web_personal`.
var WebPersonalDistFS embed.FS
