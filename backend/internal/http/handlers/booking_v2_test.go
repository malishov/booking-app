package handlers

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muradalishov/booking-app/frontend"
)

func TestBookingV2Review(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	booking := NewBooking(nil, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)))
	data := bookingPageData{
		businessForm: businessForm{Name: "Studio", Slug: "studio", Timezone: "Asia/Baku"},
		Date:         "2026-11-18",
		SelectedID:   1,
		Selected:     &serviceView{Name: "Consultation", DurationMinutes: 30, Currency: "AZN"},
		Slots:        []bookingSlot{{Value: "2026-11-18T06:30:00Z", Label: "10:30"}},
	}
	if data.reviewSlot("2026-11-18T11:00:00+04:00") {
		t.Fatal("unavailable slot accepted")
	}
	if !data.reviewSlot(data.Slots[0].Value) {
		t.Fatal("available slot rejected")
	}
	req := httptest.NewRequest("GET", "/v2/b/studio", nil)
	rec := httptest.NewRecorder()
	booking.renderBooking(rec, req, data, 200)
	if rec.Code != 200 {
		t.Fatalf("review status = %d", rec.Code)
	}
	for _, want := range []string{"/static/booking-v2.css", "/v2/b/studio/book", "Consultation", "18.11.2026", "10:30", "Asia/Baku — UTC&#43;4", "name=\"client_name\"", "name=\"phone\""} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("review missing %q", want)
		}
	}
}
