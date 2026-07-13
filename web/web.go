// Package web holds embedded frontend assets: HTML templates and static files.
package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// StaticFS holds embedded static assets, served under /static/.
//
//go:embed static
var StaticFS embed.FS

// ParseTemplates parses all embedded HTML templates.
func ParseTemplates() (*template.Template, error) {
	return template.ParseFS(templateFS, "templates/*.html")
}
