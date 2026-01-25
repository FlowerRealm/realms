package web

import "embed"

// Web 控制台模板资源。
//
//go:embed templates/*.html
var templatesFS embed.FS
