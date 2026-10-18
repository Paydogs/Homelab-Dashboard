// Package web holds the embedded UI assets (templates and static files).
package web

import "embed"

//go:embed all:static all:templates
var Assets embed.FS
