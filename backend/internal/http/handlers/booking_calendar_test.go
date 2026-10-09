package handlers

import (
	"github.com/muradalishov/booking-app/backend/internal/store"
	"testing"
	"time"
)

func TestCalendarAvailabilityAndHorizon(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Baku")
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, loc)
	settings := defaultSchedule()
	for i := range settings.Days {
		settings.Days[i].Enabled = true
	}
	settings.DaysOff = "2026-10-11"
	busy := []store.BusyInterval{{Start: time.Date(2026, 10, 10, 9, 0, 0, 0, loc), End: time.Date(2026, 10, 10, 18, 0, 0, 0, loc)}}
	months := bookingCalendar(settings, loc, 90, busy, now, "2026-11-02")
	cells := map[string]calendarDay{}
	for _, m := range months {
		if len(m.Days)%7 != 0 {
			t.Fatal("incomplete calendar week")
		}
		for _, c := range m.Days {
			if c.Number != 0 {
				cells[c.Date] = c
			}
		}
	}
	if len(months) != 3 || months[0].Name != "Октябрь" || months[0].Days[0].Number != 0 || months[0].Days[3].Number != 1 {
		t.Fatal("month alignment incorrect")
	}
	if !months[1].Selected || !cells["2026-11-02"].Selected {
		t.Fatal("selected month lost")
	}
	for _, date := range []string{"2026-10-08", "2026-10-10", "2026-10-11", "2026-12-09"} {
		if cells[date].Available {
			t.Fatalf("%s should be disabled", date)
		}
	}
	for _, date := range []string{"2026-10-09", "2026-10-12", "2026-12-08"} {
		if !cells[date].Available {
			t.Fatalf("%s should be available", date)
		}
	}
	long := bookingCalendar(settings, loc, 600, busy, now, "2026-10-09")
	for _, m := range long {
		for _, c := range m.Days {
			if c.Available {
				t.Fatal("duration not applied")
			}
		}
	}
}
