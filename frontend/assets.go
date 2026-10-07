// Package frontend holds embedded frontend assets: HTML templates and static files.
package frontend

import (
	"embed"
	"fmt"
	"html/template"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

// StaticFS holds embedded static assets, served under /static/.
//
//go:embed static
var StaticFS embed.FS

// ParseTemplates parses all embedded HTML templates.
func ParseTemplates() (*template.Template, error) {
	return template.New("frontend").Funcs(template.FuncMap{
		"timezoneLabel": timezoneLabel,
		"t":             func(text string) string { return text },
		"lang":          func() string { return "ru" },
		"currentPath":   func() string { return "/" },
	}).ParseFS(templateFS, "templates/*.html")
}

// timezoneLabel includes the current UTC offset, accounting for daylight saving.
func timezoneLabel(zone string) string {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return zone
	}
	_, offset := time.Now().In(location).Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	label := fmt.Sprintf("%s — UTC%s%d", zone, sign, offset/3600)
	if minutes := offset / 60 % 60; minutes != 0 {
		label += fmt.Sprintf(":%02d", minutes)
	}
	return label
}
