package handlers

import (
	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOwnerCalendarGeometry(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Baku")
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	settings := defaultSchedule()
	settings.Days[0].Enabled = true
	settings.Days[0].Start = "09:00"
	settings.Days[0].End = "18:00"
	settings.Days[0].BreakStart = "13:00"
	settings.Days[0].BreakEnd = "14:00"
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 12, hour, minute, 0, 0, loc) }
	rows := []store.BookingView{{ID: 1, Name: "First", Start: at(9, 0), End: at(10, 30), Status: "confirmed"}, {ID: 2, Name: "Cancelled", Start: at(11, 0), End: at(12, 0), Status: "cancelled"}, {ID: 3, Name: "No show", Start: at(12, 0), End: at(12, 30), Status: "no_show"}, {ID: 4, Name: "Completed", Start: at(15, 0), End: at(16, 0), Status: "completed"}}
	cal := buildOwnerCalendar(day, settings, rows, day)
	if cal.Closed || cal.Count != 2 || cal.Height != 540 || len(cal.Events) != 2 {
		t.Fatalf("bad day calendar: %+v", cal)
	}
	if cal.Events[0].Top != 0 || cal.Events[0].Height != 90 || cal.Events[1].Top != 360 {
		t.Fatal("visit time geometry incorrect")
	}
	if len(cal.Blocks) != 2 || cal.Blocks[1].Top != 240 || cal.Blocks[1].Height != 60 {
		t.Fatal("break position incorrect")
	}
	if len(cal.Days) != 7 || cal.Days[0].Date != "2026-10-12" || !cal.Days[0].Selected {
		t.Fatal("week navigation not Monday-based")
	}
	settings.DaysOff = "2026-10-12"
	cal = buildOwnerCalendar(day, settings, rows, day)
	if !cal.Closed || len(cal.Blocks) != 0 || cal.Count != 2 {
		t.Fatal("holiday hid existing visits or remained open")
	}
	rows = []store.BookingView{{ID: 1, Start: at(7, 30), End: at(8, 30), Status: "confirmed"}, {ID: 2, Start: at(19, 0), End: at(20, 0), Status: "confirmed"}}
	cal = buildOwnerCalendar(day, settings, rows, day)
	if cal.Events[0].Top != 30 || cal.Events[1].Top != 720 || cal.Height != 780 {
		t.Fatal("out-of-schedule visits clipped")
	}
}

func TestOwnerCalendarDSTAndMidnight(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	first := time.Date(2026, 11, 1, 5, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	rows := []store.BookingView{{ID: 1, Start: first, End: first.Add(30 * time.Minute), Status: "confirmed"}, {ID: 2, Start: second, End: second.Add(30 * time.Minute), Status: "confirmed"}}
	cal := buildOwnerCalendar(day, defaultSchedule(), rows, day)
	if cal.Events[1].Top-cal.Events[0].Top != 60 {
		t.Fatal("repeated DST hour overlapped")
	}
	repeated := 0
	for _, tick := range cal.Ticks {
		if strings.HasPrefix(tick.Label, "01:00 ") {
			repeated++
		}
	}
	if repeated != 2 {
		t.Fatal("repeated hours lack offset labels")
	}
	rows = []store.BookingView{{ID: 3, Start: day.Add(-30 * time.Minute), End: day.Add(30 * time.Minute), Status: "confirmed"}}
	cal = buildOwnerCalendar(day, defaultSchedule(), rows, day)
	if cal.Events[0].Top != 0 || cal.Events[0].Height != 30 {
		t.Fatal("previous-day visit not clipped to local midnight")
	}
	rows = append(rows, store.BookingView{ID: 4, Start: day, End: day.Add(time.Hour), Status: "confirmed"})
	cal = buildOwnerCalendar(day, defaultSchedule(), rows, day)
	if cal.Events[0].Left == cal.Events[1].Left || cal.Events[0].Width != 50 {
		t.Fatal("overlapping visits lack separate lanes")
	}
}

func TestManagementReturnPreservesSafeDate(t *testing.T) {
	if got := managementReturn("today", "2026-10-12"); got != "/cabinet/bookings?date=2026-10-12&filter=today" {
		t.Fatal(got)
	}
	for _, raw := range []string{"https://evil.test", "2026-02-30", "2026-10-12&saved=cancelled"} {
		if got := managementReturn("today", raw); got != "/cabinet/bookings?filter=today" {
			t.Fatal(got)
		}
	}
}

func TestOwnerCalendarTemplates(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Baku")
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	rows := []store.BookingView{{ID: 7, Name: "Client <test>", Phone: "+994500000000", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour), Status: "confirmed", PriceCents: 3500, ServiceName: "Manicure"}}
	page := ownerBookingsPage{Business: store.Business{Timezone: "Asia/Baku"}, Filter: "today", Calendar: buildOwnerCalendar(day, defaultSchedule(), rows, day), Bookings: ownerBookingViews(rows, loc, day), SelectedID: 7}
	for _, lang := range []string{"ru", "en", "az"} {
		req := httptest.NewRequest("GET", "/cabinet/bookings", nil)
		req.AddCookie(&http.Cookie{Name: "ui_language", Value: lang})
		rec := httptest.NewRecorder()
		renderHTML(rec, req, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)), "bookings.html", page, 200)
		if rec.Code != 200 || strings.Contains(rec.Body.String(), "ZgotmplZ") || !strings.Contains(rec.Body.String(), "data-owner-visit=\"7\"") || !strings.Contains(rec.Body.String(), "Client &lt;test&gt;") {
			t.Fatalf("calendar rendering failed in %s: %s", lang, rec.Body.String())
		}
	}
}
