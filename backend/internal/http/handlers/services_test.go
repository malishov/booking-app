package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/muradalishov/booking-app/backend/internal/store"
)

type fakeServiceStore struct {
	fakeBusinesses
	row   store.Service
	calls int
	owner int64
}

func (f *fakeServiceStore) ListServicesByOwner(_ context.Context, owner int64) ([]store.Service, error) {
	if f.row.ID == 0 || owner != f.profile.OwnerUserID {
		return nil, nil
	}
	return []store.Service{f.row}, nil
}
func (f *fakeServiceStore) GetServiceByOwner(_ context.Context, p store.GetServiceByOwnerParams) (store.Service, error) {
	if p.ID != f.row.ID || p.OwnerUserID != f.profile.OwnerUserID {
		return store.Service{}, pgx.ErrNoRows
	}
	return f.row, nil
}
func (f *fakeServiceStore) CreateService(_ context.Context, p store.CreateServiceParams) (store.Service, error) {
	f.calls++
	f.owner = p.OwnerUserID
	f.row = store.Service{ID: 1, BusinessID: f.profile.ID, Name: p.Name, DurationMinutes: p.DurationMinutes, PriceCents: p.PriceCents, Currency: p.Currency, IsActive: true}
	return f.row, nil
}
func (f *fakeServiceStore) UpdateService(ctx context.Context, p store.UpdateServiceParams) (store.Service, error) {
	if _, err := f.GetServiceByOwner(ctx, store.GetServiceByOwnerParams{ID: p.ID, OwnerUserID: p.OwnerUserID}); err != nil {
		return store.Service{}, err
	}
	f.calls++
	f.owner = p.OwnerUserID
	f.row.Name = p.Name
	f.row.DurationMinutes = p.DurationMinutes
	f.row.PriceCents = p.PriceCents
	f.row.Currency = p.Currency
	return f.row, nil
}
func (f *fakeServiceStore) ToggleService(ctx context.Context, p store.ToggleServiceParams) (store.Service, error) {
	if _, err := f.GetServiceByOwner(ctx, store.GetServiceByOwnerParams{ID: p.ID, OwnerUserID: p.OwnerUserID}); err != nil {
		return store.Service{}, err
	}
	f.calls++
	f.row.IsActive = !f.row.IsActive
	return f.row, nil
}

func TestPricePrecisionAndValidation(t *testing.T) {
	for _, tc := range []struct {
		price string
		cents int32
		valid bool
	}{
		{"0", 0, true}, {"0.01", 1, true}, {"35.10", 3510, true}, {"35,1", 3510, true}, {"9999999.99", 999999999, true},
		{"-1", 0, false}, {"1.001", 0, false}, {"1e3", 0, false}, {"NaN", 0, false}, {"10000000", 0, false}, {"", 0, false},
	} {
		_, cents, message := validateService(serviceForm{Name: "Маникюр", Duration: "90", Price: tc.price, Currency: "AZN"})
		if (message == "") != tc.valid || tc.valid && cents != tc.cents {
			t.Fatalf("price %q: %d %q", tc.price, cents, message)
		}
	}
	for _, form := range []serviceForm{{Name: "x", Duration: "90", Price: "35", Currency: "AZN"}, {Name: "Маникюр", Duration: "0", Price: "35", Currency: "AZN"}, {Name: "Маникюр", Duration: "1.5", Price: "35", Currency: "AZN"}, {Name: "Маникюр", Duration: "1441", Price: "35", Currency: "AZN"}, {Name: "Маникюр", Duration: "90", Price: "35", Currency: "XXX"}} {
		if _, _, message := validateService(form); message == "" {
			t.Fatal("invalid service accepted")
		}
	}
}

func TestServiceLifecycleAndOwnership(t *testing.T) {
	f := &fakeServiceStore{fakeBusinesses: fakeBusinesses{exists: true, profile: store.Business{ID: 1, OwnerUserID: 42, Slug: "anna"}}}
	base := testBusiness(t, &f.fakeBusinesses)
	s := NewServices(f, base.tmpl, base.logger)
	router := chi.NewRouter()
	router.Post("/services", s.Save)
	router.Post("/services/{id}/edit", s.Save)
	router.Post("/services/{id}/toggle", s.Toggle)
	values := url.Values{"name": {"Маникюр"}, "duration": {"90"}, "price": {"35,10"}, "currency": {"AZN"}, "owner_user_id": {"999"}, "business_id": {"999"}}
	request := func(path string) int {
		req := ownerRequest(postForm(values.Encode()))
		req.URL.Path = path
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	if request("/services") != http.StatusSeeOther || f.row.PriceCents != 3510 || f.owner != 42 {
		t.Fatal("incorrect create or ownership")
	}
	values.Set("price", "40.99")
	if request("/services/1/edit") != http.StatusSeeOther || f.row.PriceCents != 4099 {
		t.Fatal("edit failed")
	}
	if request("/services/1/toggle") != http.StatusSeeOther || f.row.IsActive {
		t.Fatal("hide failed")
	}
	if request("/services/1/toggle") != http.StatusSeeOther || !f.row.IsActive {
		t.Fatal("restore failed")
	}
	calls := f.calls
	if request("/services/999/edit") != http.StatusNotFound || request("/services/999/toggle") != http.StatusNotFound || f.calls != calls {
		t.Fatal("foreign service was mutated")
	}
	values.Set("price", "1.001")
	if request("/services/1/edit") != http.StatusBadRequest || f.calls != calls {
		t.Fatal("invalid edit was saved")
	}
	page := httptest.NewRecorder()
	req := ownerRequest(httptest.NewRequest(http.MethodGet, "/cabinet/services", nil))
	req.AddCookie(&http.Cookie{Name: languageCookie, Value: "az"})
	s.Page(page, req)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Xidmətlər və qiymətlər") || !strings.Contains(page.Body.String(), "40.99") {
		t.Fatal("catalog not localized or rendered")
	}
}

func TestServicesRequireBusiness(t *testing.T) {
	f := &fakeServiceStore{}
	base := testBusiness(t, &f.fakeBusinesses)
	s := NewServices(f, base.tmpl, base.logger)
	rec := httptest.NewRecorder()
	s.Page(rec, ownerRequest(httptest.NewRequest(http.MethodGet, "/cabinet/services", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/cabinet?setup=services#profile-editor" {
		t.Fatal("missing business was not redirected")
	}
	guest := httptest.NewRecorder()
	s.Save(guest, postForm(""))
	if guest.Code != http.StatusSeeOther || guest.Header().Get("Location") != "/login" {
		t.Fatal("guest can mutate service")
	}
}
