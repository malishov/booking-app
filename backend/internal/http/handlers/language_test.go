package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLanguagePreferenceAndLocalRedirect(t *testing.T) {
	for _, target := range []string{"/cabinet", "/login?next=1", "https://evil.example", "//evil.example", `/\evil.example`} {
		rec := httptest.NewRecorder()
		SetLanguage(true)(rec, postForm(url.Values{"language": {"en"}, "return_to": {target}}.Encode()))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d", rec.Code)
		}
		expected := target
		if strings.Contains(target, "evil.example") {
			expected = "/"
		}
		if rec.Header().Get("Location") != expected {
			t.Fatal("unsafe or incorrect redirect")
		}
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Value != "en" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" || cookies[0].MaxAge <= 0 {
			t.Fatal("language preference cookie missing or invalid")
		}
	}
	rec := httptest.NewRecorder()
	SetLanguage(false)(rec, postForm("language=invalid&return_to=/cabinet"))
	if rec.Code != http.StatusBadRequest || len(rec.Result().Cookies()) != 0 {
		t.Fatal("unsupported language persisted")
	}
}

func TestInterfaceLanguagesRemainIsolated(t *testing.T) {
	h := newAuth(t, &fakeUsers{})
	for _, language := range []string{"en", "ru", "az", "en", "unknown"} {
		req := httptest.NewRequest(http.MethodGet, "/login", nil)
		req.AddCookie(&http.Cookie{Name: languageCookie, Value: language})
		rec := httptest.NewRecorder()
		h.LoginForm(rec, req)
		expectedLang, expectedHeading := "ru", "С возвращением."
		if language == "en" {
			expectedLang, expectedHeading = "en", "Welcome back."
		}
		if language == "az" {
			expectedLang, expectedHeading = "az", "Yenidən xoş gəlmisiniz."
		}
		body := rec.Body.String()
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Language") != expectedLang || !strings.Contains(body, `lang="`+expectedLang+`"`) || !strings.Contains(body, expectedHeading) || !strings.Contains(body, ">окно<span") {
			t.Fatalf("incorrect localized page for %s", language)
		}
	}
	req := postForm("email=wrong&password=longenough")
	req.AddCookie(&http.Cookie{Name: languageCookie, Value: "en"})
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Enter a valid email address.") {
		t.Fatal("validation error was not translated")
	}
}

func TestLanguageDoesNotTranslateBusinessContent(t *testing.T) {
	s := &fakeBusinesses{}
	b := testBusiness(t, s)
	values := validProfileValues()
	s.exists = true
	s.profile.Name = values.Get("name")
	s.profile.Description = values.Get("description")
	s.profile.Slug = "anna-nails"
	s.profile.Timezone = "Asia/Dubai"
	req := ownerRequest(httptest.NewRequest(http.MethodGet, "/cabinet", nil))
	req.AddCookie(&http.Cookie{Name: languageCookie, Value: "en"})
	rec := httptest.NewRecorder()
	b.Cabinet(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Save changes") || !strings.Contains(body, "Анна · маникюр") || !strings.Contains(body, "Бережно и красиво.") {
		t.Fatal("interface or business content changed incorrectly")
	}
}
