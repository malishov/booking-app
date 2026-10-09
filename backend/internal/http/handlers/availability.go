package handlers

import (
	"github.com/muradalishov/booking-app/backend/internal/store"
	"strings"
	"time"
)

const bookingHorizon = 60

type bookingSlot struct{ Value, Label string }

// availableSlots uses local wall-clock times and excludes DST gaps, breaks,
// past times, extra days off and overlapping bookings. Slots use a 15-minute
// cadence from shift start; the complete service must fit inside a work interval.
func availableSlots(settings scheduleSettings, date string, location *time.Location, duration int32, busy []store.BusyInterval, now time.Time) []bookingSlot {
	result := []bookingSlot{}
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil || duration <= 0 || len(settings.Days) != 7 {
		return result
	}
	today := time.Date(now.In(location).Year(), now.In(location).Month(), now.In(location).Day(), 0, 0, 0, 0, location)
	if day.Before(today) || day.After(today.AddDate(0, 0, bookingHorizon)) {
		return result
	}
	for _, off := range strings.Split(settings.DaysOff, "\n") {
		if strings.TrimSpace(off) == date {
			return result
		}
	}
	d := settings.Days[(int(day.Weekday())+6)%7]
	if !d.Enabled {
		return result
	}
	parse := func(value string) (time.Time, bool) {
		t, e := time.ParseInLocation("2006-01-02 15:04", date+" "+value, location)
		return t, e == nil && t.Format("2006-01-02 15:04") == date+" "+value
	}
	start, ok1 := parse(d.Start)
	end, ok2 := parse(d.End)
	if !ok1 || !ok2 || !start.Before(end) {
		return result
	}
	var bs, be time.Time
	if d.BreakStart != "" || d.BreakEnd != "" {
		var a, b bool
		bs, a = parse(d.BreakStart)
		be, b = parse(d.BreakEnd)
		if !a || !b || !bs.Before(be) {
			return result
		}
	}
	for t := start; t.Before(end); t = t.Add(15 * time.Minute) {
		finish := t.Add(time.Duration(duration) * time.Minute)
		if finish.After(end) || !t.After(now) || (!bs.IsZero() && t.Before(be) && finish.After(bs)) {
			continue
		}
		available := true
		for _, b := range busy {
			if t.Before(b.End) && finish.After(b.Start) {
				available = false
				break
			}
		}
		if available {
			result = append(result, bookingSlot{Value: t.UTC().Format(time.RFC3339), Label: t.Format("15:04")})
		}
	}
	labels := map[string]int{}
	for _, slot := range result {
		labels[slot.Label]++
	}
	for i := range result {
		if labels[result[i].Label] > 1 {
			at, _ := time.Parse(time.RFC3339, result[i].Value)
			result[i].Label += " (" + at.In(location).Format("-07:00") + ")"
		}
	}
	return result
}
