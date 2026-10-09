package handlers

import (
	"context"
	"encoding/json"
	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fakeSchedule struct {
	fakeBusinesses
	raw   []byte
	owner int64
	saves int
}

func (f *fakeSchedule) GetSchedule(context.Context, int64) ([]byte, error) {
	if f.raw == nil {
		return []byte("{}"), nil
	}
	return f.raw, nil
}
func (f *fakeSchedule) SaveSchedule(_ context.Context, owner int64, raw []byte) error {
	f.owner = owner
	f.raw = raw
	f.saves++
	return nil
}

func TestScheduleValidation(t *testing.T) {
	for _, tc := range []struct {
		name, start, end, bs, be, dates string
		bad                             bool
	}{
		{"working", "09:00", "18:00", "13:00", "14:00", "2026-12-31", false},
		{"no break", "09:00", "18:00", "", "", "", false},
		{"overnight", "22:00", "06:00", "", "", "", true},
		{"invalid time", "99:00", "18:00", "", "", "", true},
		{"empty shift", "09:00", "09:00", "", "", "", true},
		{"half break", "09:00", "18:00", "13:00", "", "", true},
		{"break outside", "09:00", "18:00", "08:00", "10:00", "", true},
		{"invalid date", "09:00", "18:00", "", "", "2026-02-30", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := defaultSchedule()
			s.Days[0].Enabled = true
			s.Days[0].Start = tc.start
			s.Days[0].End = tc.end
			s.Days[0].BreakStart = tc.bs
			s.Days[0].BreakEnd = tc.be
			s.DaysOff = tc.dates
			if (validateSchedule(&s) != "") != tc.bad {
				t.Fatal("incorrect validation")
			}
		})
	}
	s := defaultSchedule()
	s.DaysOff = "2026-12-31, 2026-12-31\n2027-01-01"
	if validateSchedule(&s) != "" || s.DaysOff != "2026-12-31\n2027-01-01" {
		t.Fatal("dates not normalized")
	}
}
func TestScheduleSaveAndRender(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSchedule{fakeBusinesses: fakeBusinesses{exists: true, profile: store.Business{ID: 4, OwnerUserID: 7, Timezone: "Asia/Baku"}}}
	h := NewSchedule(f, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)))
	values := url.Values{"day_0_enabled": {"on"}, "day_0_start": {"09:00"}, "day_0_end": {"18:00"}, "day_0_break_start": {"13:00"}, "day_0_break_end": {"14:00"}, "days_off": {"2026-12-31"}, "owner_user_id": {"999"}}
	request := func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/cabinet/schedule", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r = r.WithContext(context.WithValue(r.Context(), sessionUserKey{}, store.GetSessionUserRow{ID: 7, Email: "demo@example.test"}))
		w := httptest.NewRecorder()
		if method == "POST" {
			h.Save(w, r)
		} else {
			h.Page(w, r)
		}
		return w
	}
	if w := request("POST", values.Encode()); w.Code != 303 || f.owner != 7 || f.saves != 1 {
		t.Fatalf("save: %d", w.Code)
	}
	var saved scheduleSettings
	if err := json.Unmarshal(f.raw, &saved); err != nil || !saved.Days[0].Enabled || saved.DaysOff != "2026-12-31" {
		t.Fatal("not persisted")
	}
	if w := request("GET", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Asia/Baku — UTC&#43;4") {
		t.Fatalf("render: %d", w.Code)
	}
	values.Set("day_0_end", "08:00")
	if w := request("POST", values.Encode()); w.Code != 400 || f.saves != 1 {
		t.Fatal("invalid schedule persisted")
	}
	w := httptest.NewRecorder()
	h.Page(w, httptest.NewRequest("GET", "/cabinet/schedule", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/login" {
		t.Fatal("guest allowed")
	}
	f.exists = false
	if w := request("GET", ""); w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "setup=schedule") {
		t.Fatal("missing profile allowed")
	}
}
