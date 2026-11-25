package handlers

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
)

func TestPublicBookingUsesNewDesign(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	b := NewBooking(nil, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)))
	data := bookingPageData{businessForm: businessForm{Name: "Studio", Slug: "studio", Timezone: "Asia/Baku"}, Date: "2026-11-23", SelectedID: 1, Selected: &serviceView{Name: "Service", DurationMinutes: 45}, Services: []serviceView{{ID: 1, Name: "Service", DurationMinutes: 45}}, Slots: []bookingSlot{{Value: "2026-11-23T10:00:00Z", Label: "14:00"}}, Calendar: []calendarMonth{{Name: "Ноябрь", Year: 2026, Days: []calendarDay{{Number: 23, Date: "2026-11-23", Count: 1, Available: true}}}}}
	for _, path := range []string{"/b/studio", "/v2/b/studio"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			b.renderBooking(rec, httptest.NewRequest("GET", path+"?service=1", nil), data, 200)
			for _, want := range []string{"/static/booking-v2.js", "data-count=\"1\"", "data-slot-form", "data-date-tools", "data-periods", "data-more-slots", "action=\"" + path + "\""} {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
	for _, query := range []string{"", "?step=service&service=1&date=2026-11-23"} {
		rec := httptest.NewRecorder()
		b.renderBooking(rec, httptest.NewRequest("GET", "/b/studio"+query, nil), data, 200)
		body := rec.Body.String()
		if !strings.Contains(body, "v2-service-options") || !strings.Contains(body, "Выбрать время") {
			t.Error("service step must show service choices and continue action")
		}
		if strings.Contains(body, "data-slot-form") || strings.Contains(body, "data-date-tools") {
			t.Error("service step must not show dates or slots")
		}
	}
	if !data.reviewSlot(data.Slots[0].Value) {
		t.Fatal("review rejected")
	}
	rec := httptest.NewRecorder()
	b.renderBooking(rec, httptest.NewRequest("GET", "/b/studio", nil), data, 200)
	for _, want := range []string{"/b/studio/book", "14:00–14:45", "client_name", "phone"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("review missing %q", want)
		}
	}
}

func TestReceiptCelebratesOnlyConfirmedBookings(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	b := NewBooking(nil, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, status := range []string{"confirmed", "cancelled", "completed", "no_show"} {
		t.Run(status, func(t *testing.T) {
			start := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
			data := bookingPageData{businessForm: businessForm{Name: "Studio", Slug: "studio", Timezone: "Asia/Baku"}, Receipt: &store.BookingView{Start: start, End: start.Add(45 * time.Minute), Status: status, ServiceName: "Care"}, ReceiptTime: "14:00"}
			rec := httptest.NewRecorder()
			b.renderBooking(rec, httptest.NewRequest("GET", "/booked/secret", nil), data, 200)
			body := rec.Body.String()
			if got := strings.Contains(body, "v2-emblem"); got != (status == "confirmed") {
				t.Errorf("success emblem for %s = %v", status, got)
			}
			if got := strings.Contains(body, "data-save-calendar"); got != (status == "confirmed") {
				t.Errorf("calendar action for %s = %v", status, got)
			}
			if !strings.Contains(body, "data-copy-receipt") {
				t.Fatal("missing private link action")
			}
			if !strings.Contains(body, "data-link-fallback hidden") {
				t.Error("receipt URL must be a hidden copy fallback")
			}
			if status == "confirmed" {
				for _, want := range []string{"<p>23 ноября · 14:00</p>", "14:00–14:45", "20261123T100000Z", "20261123T104500Z"} {
					if !strings.Contains(body, want) {
						t.Errorf("missing %s", want)
					}
				}
			}
		})
	}
}

func TestBookingShortDateLanguage(t *testing.T) {
	date := time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC)
	for language, want := range map[string]string{"ru": "23 ноября", "en": "23 November", "az": "23 noyabr"} {
		if got := bookingShortDate(date, language); got != want {
			t.Errorf("%s: got %q, want %q", language, got, want)
		}
	}
}
