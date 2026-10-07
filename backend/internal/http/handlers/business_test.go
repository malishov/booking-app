package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
)

type fakeBusinesses struct {
	profile         store.Business
	exists          bool
	saveErr, getErr error
	saved           store.SaveBusinessParams
	calls           int
	ownerID         int64
}

func (f *fakeBusinesses) ListPublicServices(_ context.Context, _ int64) ([]store.Service, error) {
	return nil, nil
}

func (f *fakeBusinesses) GetBusinessByOwner(_ context.Context, id int64) (store.Business, error) {
	f.ownerID = id
	if f.getErr != nil {
		return store.Business{}, f.getErr
	}
	if !f.exists {
		return store.Business{}, pgx.ErrNoRows
	}
	return f.profile, nil
}

func (f *fakeBusinesses) GetBusinessBySlug(_ context.Context, slug string) (store.Business, error) {
	if f.getErr != nil {
		return store.Business{}, f.getErr
	}
	if !f.exists || slug != f.profile.Slug {
		return store.Business{}, pgx.ErrNoRows
	}
	return f.profile, nil
}

func (f *fakeBusinesses) SaveBusiness(_ context.Context, p store.SaveBusinessParams) (store.Business, error) {
	f.calls++
	f.saved = p
	if f.saveErr != nil {
		return store.Business{}, f.saveErr
	}
	f.exists = true
	f.profile = store.Business{ID: 1, OwnerUserID: p.OwnerUserID, Name: p.Name, Slug: p.Slug, Description: p.Description, Timezone: p.Timezone}
	return f.profile, nil
}

func testBusiness(t *testing.T, s BusinessStore) *Business {
	t.Helper()
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	return NewBusiness(s, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func ownerRequest(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), sessionUserKey{}, store.GetSessionUserRow{ID: 42, Email: "private@example.com"}))
}

func validProfileValues() url.Values {
	return url.Values{"name": {"Анна · маникюр"}, "slug": {"anna-nails"}, "description": {"Бережно и красиво."}, "timezone": {"Asia/Dubai"}}
}

func TestBusinessCreateAndEdit(t *testing.T) {
	s := &fakeBusinesses{}
	b := testBusiness(t, s)
	values := validProfileValues()
	values.Set("owner_user_id", "999")
	for _, name := range []string{"Анна · маникюр", "Студия Анны"} {
		values.Set("name", name)
		values.Set("slug", " ANNA-NAILS ")
		rec := httptest.NewRecorder()
		b.SaveProfile(rec, ownerRequest(postForm(values.Encode())))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/cabinet?saved=1" {
			t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
		}
		if s.saved.OwnerUserID != 42 || s.ownerID != 42 || s.saved.Name != name || s.saved.Slug != "anna-nails" {
			t.Fatalf("incorrect owner or data: %+v", s.saved)
		}
		page := httptest.NewRecorder()
		b.Cabinet(page, ownerRequest(httptest.NewRequest(http.MethodGet, "/cabinet?saved=1", nil)))
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), name) || !strings.Contains(page.Body.String(), "/b/anna-nails") || !strings.Contains(page.Body.String(), "Изменения сохранены") {
			t.Fatal("cabinet did not render saved profile")
		}
	}
}

func TestBusinessValidationDoesNotSave(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"name", ""}, {"name", strings.Repeat("я", 101)}, {"slug", "ab"}, {"slug", "../admin"},
		{"slug", "anna--nails"}, {"slug", "анна"}, {"slug", strings.Repeat("a", 61)},
		{"description", strings.Repeat("я", 1001)}, {"timezone", ""}, {"timezone", "Local"}, {"timezone", "not/a/timezone"},
	} {
		t.Run(tc.field+tc.value[:min(10, len(tc.value))], func(t *testing.T) {
			s := &fakeBusinesses{}
			b := testBusiness(t, s)
			values := validProfileValues()
			values.Set(tc.field, tc.value)
			rec := httptest.NewRecorder()
			b.SaveProfile(rec, ownerRequest(postForm(values.Encode())))
			if rec.Code != http.StatusBadRequest || s.calls != 0 || !strings.Contains(rec.Body.String(), `role="alert"`) {
				t.Fatalf("invalid input saved: %d", rec.Code)
			}
		})
	}
}

func TestBusinessSlugConflict(t *testing.T) {
	s := &fakeBusinesses{saveErr: &pgconn.PgError{Code: "23505", ConstraintName: "businesses_slug_key"}}
	rec := httptest.NewRecorder()
	testBusiness(t, s).SaveProfile(rec, ownerRequest(postForm(validProfileValues().Encode())))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Этот адрес уже занят") || !strings.Contains(rec.Body.String(), "Бережно и красиво") {
		t.Fatal("conflict did not preserve form")
	}
}

func TestBusinessPublicPage(t *testing.T) {
	s := &fakeBusinesses{exists: true, profile: store.Business{OwnerUserID: 42, Name: "Анна <script>alert(1)</script>", Slug: "anna-nails", Description: "<img src=x onerror=alert(1)>", Timezone: "Asia/Dubai"}}
	b := testBusiness(t, s)
	router := chi.NewRouter()
	router.Get("/b/{slug}", b.PublicPage)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/b/anna-nails", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "<script>alert") || strings.Contains(body, "<img src=x") || strings.Contains(body, "private@example.com") || strings.Contains(body, "owner_user_id") {
		t.Fatal("public page exposed unsafe or private data")
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "Asia/Dubai") {
		t.Fatal("public profile data missing")
	}
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/b/missing", nil))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "адрес изменился") {
		t.Fatal("missing page must be 404")
	}
}

func TestBusinessGuestsAndFailures(t *testing.T) {
	b := testBusiness(t, &fakeBusinesses{})
	for _, handler := range []http.HandlerFunc{b.Cabinet, b.SaveProfile} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/cabinet", nil))
		if rec.Code != http.StatusSeeOther {
			t.Fatal("guest accessed cabinet")
		}
	}
	for _, s := range []*fakeBusinesses{{getErr: errors.New("offline")}, {saveErr: errors.New("offline")}} {
		rec := httptest.NewRecorder()
		testBusiness(t, s).SaveProfile(rec, ownerRequest(postForm(validProfileValues().Encode())))
		if rec.Code != http.StatusInternalServerError || rec.Header().Get("Location") != "" {
			t.Fatal("DB failure must not report success")
		}
	}
	rec := httptest.NewRecorder()
	b.SaveProfile(rec, ownerRequest(postForm("name="+strings.Repeat("a", 20000))))
	if rec.Code != http.StatusBadRequest {
		t.Fatal("oversized form accepted")
	}
}
