package frontend

import (
	_ "embed"
	"encoding/json"
	"html/template"
)

//go:embed locales/en.json
var englishJSON []byte

//go:embed locales/az.json
var azerbaijaniJSON []byte

var englishTranslations map[string]string
var azerbaijaniTranslations map[string]string

func init() {
	if err := json.Unmarshal(englishJSON, &englishTranslations); err != nil {
		panic("invalid English translation catalog: " + err.Error())
	}
	if err := json.Unmarshal(azerbaijaniJSON, &azerbaijaniTranslations); err != nil {
		panic("invalid Azerbaijani translation catalog: " + err.Error())
	}
}

// Translate resolves interface copy; user content and the brand are not translated.
func Translate(language, text string) string {
	catalog := englishTranslations
	if language == "az" {
		catalog = azerbaijaniTranslations
	}
	if language == "en" || language == "az" {
		if translated, ok := catalog[text]; ok {
			return translated
		}
	}
	return text
}

// LocalizedTemplates clones the shared templates to keep request languages isolated.
func LocalizedTemplates(base *template.Template, language, path string) (*template.Template, error) {
	tmpl, err := base.Clone()
	if err != nil {
		return nil, err
	}
	return tmpl.Funcs(template.FuncMap{
		"t":           func(text string) string { return Translate(language, text) },
		"lang":        func() string { return language },
		"currentPath": func() string { return path },
	}), nil
}
