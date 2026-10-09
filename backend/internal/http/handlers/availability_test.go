package handlers

import (
	"github.com/muradalishov/booking-app/backend/internal/store"
	"testing"
	"time"
)

func TestAvailableSlots(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Baku")
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, loc)
	settings := defaultSchedule()
	d := &settings.Days[4]
	d.Enabled = true
	d.BreakStart = "13:00"
	d.BreakEnd = "14:00"
	busy := []store.BusyInterval{{Start: time.Date(2026, 10, 9, 14, 0, 0, 0, loc), End: time.Date(2026, 10, 9, 15, 0, 0, 0, loc)}}
	slots := availableSlots(settings, "2026-10-09", loc, 90, busy, now)
	labels := map[string]string{}
	for _, s := range slots {
		labels[s.Label] = s.Value
	}
	for _, label := range []string{"09:00", "11:30", "15:00", "16:30"} {
		if labels[label] == "" {
			t.Fatalf("missing %s", label)
		}
	}
	for _, label := range []string{"12:00", "13:00", "14:00", "16:45"} {
		if labels[label] != "" {
			t.Fatalf("invalid %s", label)
		}
	}
	if labels["09:00"] != "2026-10-09T05:00:00Z" {
		t.Fatal("wrong time zone")
	}
	if got := availableSlots(settings, "2026-10-10", loc, 90, nil, now); len(got) != 0 {
		t.Fatal("weekend available")
	}
	settings.DaysOff = "2026-10-09"
	if len(availableSlots(settings, "2026-10-09", loc, 90, nil, now)) != 0 {
		t.Fatal("day off available")
	}
	settings.DaysOff = ""
	now = time.Date(2026, 10, 9, 9, 30, 0, 0, loc)
	slots = availableSlots(settings, "2026-10-09", loc, 90, nil, now)
	if len(slots) == 0 || slots[0].Label != "09:45" {
		t.Fatal("past slot offered")
	}
	if len(availableSlots(settings, "2026-10-08", loc, 90, nil, now)) != 0 || len(availableSlots(settings, "2027-10-09", loc, 90, nil, now)) != 0 {
		t.Fatal("horizon ignored")
	}
	if len(availableSlots(settings, "2026-10-09", loc, 600, nil, now)) != 0 {
		t.Fatal("service extends past hours")
	}
}
func TestAvailabilityDST(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	s := defaultSchedule()
	s.Days[6].Enabled = true
	s.Days[6].Start = "01:00"
	s.Days[6].End = "04:00"
	slots := availableSlots(s, "2026-03-08", loc, 30, nil, time.Date(2026, 3, 1, 0, 0, 0, 0, loc))
	if len(slots) == 0 {
		t.Fatal("no DST slots")
	}
	for _, slot := range slots {
		start, err := time.Parse(time.RFC3339, slot.Value)
		if err != nil || start.In(loc).Hour() == 2 {
			t.Fatal("nonexistent local time offered")
		}
	}
	s.Days[6].Start = "02:30"
	if len(availableSlots(s, "2026-03-08", loc, 30, nil, time.Date(2026, 3, 1, 0, 0, 0, 0, loc))) != 0 {
		t.Fatal("DST gap shift start accepted")
	}
}
func TestBookingClientValidation(t *testing.T) {
	name, phone, err := normalizeClient(" Анна ", "+994 (50) 123-45-67")
	if err != "" || name != "Анна" || phone != "+994501234567" {
		t.Fatal("normalization")
	}
	for _, p := range []string{"123", "++994501234567", "phone", "1234567890123456"} {
		if _, _, e := normalizeClient("Анна", p); e == "" {
			t.Fatal("invalid phone accepted")
		}
	}
	if _, _, e := normalizeClient("А", "+994501234567"); e == "" {
		t.Fatal("invalid name accepted")
	}
}

func TestRepeatedDSTTimesAreDistinct(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	settings := defaultSchedule()
	settings.Days[6].Enabled = true
	settings.Days[6].Start = "00:00"
	settings.Days[6].End = "04:00"
	slots := availableSlots(settings, "2026-11-01", loc, 30, nil, time.Date(2026, 10, 9, 0, 0, 0, 0, loc))
	labels := map[string]bool{}
	for _, slot := range slots {
		if labels[slot.Label] {
			t.Fatal("ambiguous repeated local time")
		}
		labels[slot.Label] = true
	}
	if !labels["01:00 (-04:00)"] || !labels["01:00 (-05:00)"] {
		t.Fatal("repeated hour not distinguished")
	}
	if bookingTimezoneLabel("America/New_York", time.Date(2026, 11, 2, 12, 0, 0, 0, loc)) != "America/New_York — UTC-5" {
		t.Fatal("wrong future offset")
	}
}
