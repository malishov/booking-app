package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const languageCookie = "ui_language"

func requestLanguage(r *http.Request) string {
	if cookie, err := r.Cookie(languageCookie); err == nil && (cookie.Value == "en" || cookie.Value == "az") {
		return cookie.Value
	}
	return "ru"
}

// SetLanguage saves a supported interface language and returns to a local page.
func SetLanguage(secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		language := r.PostFormValue("language")
		if language != "ru" && language != "en" && language != "az" {
			http.Error(w, "Unsupported language", http.StatusBadRequest)
			return
		}
		path := r.PostFormValue("return_to")
		target, err := url.Parse(path)
		if err != nil || target.IsAbs() || target.Host != "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, `\`) {
			path = "/"
		}
		http.SetCookie(w, &http.Cookie{Name: languageCookie, Value: language, Path: "/", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 60 * 60, Expires: time.Now().Add(365 * 24 * time.Hour)})
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, path, http.StatusSeeOther)
	}
}
