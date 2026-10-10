package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muradalishov/booking-app/backend/internal/store"
)

type ownerCalendarDay struct {
	Date, Label, Weekday string
	Selected, Today      bool
}
type ownerCalendarTick struct {
	Label string
	Top   int
}
type ownerCalendarBlock struct {
	Label, Kind string
	Top, Height int
}
type ownerCalendarEvent struct {
	ID                              int64
	Name, Time, Status, StatusLabel string
	Top, Height, Left, Width        int
}
type ownerDayCalendar struct {
	Date, Label, Previous, Next, PreviousWeek, NextWeek, Today string
	Days                                                       []ownerCalendarDay
	Ticks                                                      []ownerCalendarTick
	Blocks                                                     []ownerCalendarBlock
	Events                                                     []ownerCalendarEvent
	Height, Count                                              int
	Closed                                                     bool
}
type ownerBookingsPage struct {
	Email                       string
	Business                    store.Business
	Bookings                    []ownerBookingView
	Filter, Saved, ErrorMessage string
	Error                       bool
	Calendar                    ownerDayCalendar
	SelectedID                  int64
}

func managementReturn(filter, date string) string {
	values := url.Values{"filter": {bookingFilter(filter)}}
	if filter == "today" {
		if at, err := time.Parse("2006-01-02", date); err == nil && at.Format("2006-01-02") == date {
			values.Set("date", date)
		}
	}
	return "/cabinet/bookings?" + values.Encode()
}

// Vertical positions use elapsed time, so DST repeats/gaps do not overlap visits.
func buildOwnerCalendar(day time.Time, settings scheduleSettings, rows []store.BookingView, now time.Time) ownerDayCalendar {
	loc := day.Location()
	today := now.In(loc).Format("2006-01-02")
	cal := ownerDayCalendar{Date: day.Format("2006-01-02"), Label: day.Format("02.01.2006"), Previous: day.AddDate(0, 0, -1).Format("2006-01-02"), Next: day.AddDate(0, 0, 1).Format("2006-01-02"), PreviousWeek: day.AddDate(0, 0, -7).Format("2006-01-02"), NextWeek: day.AddDate(0, 0, 7).Format("2006-01-02"), Today: today, Closed: true}
	monday := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	weekdays := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	for i := 0; i < 7; i++ {
		d := monday.AddDate(0, 0, i)
		date := d.Format("2006-01-02")
		cal.Days = append(cal.Days, ownerCalendarDay{Date: date, Label: d.Format("02"), Weekday: weekdays[i], Selected: date == cal.Date, Today: date == today})
	}
	nextDay := day.AddDate(0, 0, 1)
	start := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), 18, 0, 0, 0, loc)
	parse := func(value string) time.Time {
		at, err := time.ParseInLocation("2006-01-02 15:04", cal.Date+" "+value, loc)
		if err != nil || at.Format("2006-01-02 15:04") != cal.Date+" "+value {
			return time.Time{}
		}
		return at
	}
	var workStart, workEnd, breakStart, breakEnd time.Time
	if len(settings.Days) == 7 {
		d := settings.Days[(int(day.Weekday())+6)%7]
		off := false
		for _, date := range strings.Split(settings.DaysOff, "\n") {
			off = off || strings.TrimSpace(date) == cal.Date
		}
		if d.Enabled && !off {
			workStart = parse(d.Start)
			workEnd = parse(d.End)
			if !workStart.IsZero() && workEnd.After(workStart) {
				cal.Closed = false
				start = workStart
				end = workEnd
				breakStart = parse(d.BreakStart)
				breakEnd = parse(d.BreakEnd)
			}
		}
	}
	active := []store.BookingView{}
	for _, r := range rows {
		if r.Status == "cancelled" || r.Status == "no_show" {
			continue
		}
		active = append(active, r)
		a, z := r.Start, r.End
		if a.Before(day) {
			a = day
		}
		if z.After(nextDay) {
			z = nextDay
		}
		if a.Before(start) {
			start = a
		}
		if z.After(end) {
			end = z
		}
	}
	// Round the visible range to full elapsed hours within the selected local date.
	elapsed := int(start.Sub(day) / time.Minute)
	start = day.Add(time.Duration(elapsed/60) * time.Hour)
	minutes := int(end.Sub(day) / time.Minute)
	if minutes%60 != 0 {
		minutes = (minutes/60 + 1) * 60
	}
	end = day.Add(time.Duration(minutes) * time.Minute)
	if end.After(nextDay) {
		end = nextDay
	}
	const pixelsPerHour = 60
	pos := func(at time.Time) int { return int(at.Sub(start).Seconds() * pixelsPerHour / 3600) }
	cal.Height = pos(end)
	if cal.Height < pixelsPerHour {
		cal.Height = pixelsPerHour
	}
	seen := map[string]int{}
	for at := start; !at.After(end); at = at.Add(time.Hour) {
		label := at.In(loc).Format("15:04")
		seen[label]++
	}
	for at := start; !at.After(end); at = at.Add(time.Hour) {
		label := at.In(loc).Format("15:04")
		if seen[label] > 1 {
			label += " " + at.In(loc).Format("-07:00")
		}
		cal.Ticks = append(cal.Ticks, ownerCalendarTick{Label: label, Top: pos(at)})
	}
	if !cal.Closed {
		cal.Blocks = append(cal.Blocks, ownerCalendarBlock{Label: "Рабочее время", Kind: "work", Top: pos(workStart), Height: pos(workEnd) - pos(workStart)})
		if !breakStart.IsZero() && breakEnd.After(breakStart) {
			cal.Blocks = append(cal.Blocks, ownerCalendarBlock{Label: "Перерыв", Kind: "break", Top: pos(breakStart), Height: pos(breakEnd) - pos(breakStart)})
		}
	}
	// Allocate lanes for appointments that overlap in a historical/timezone view.
	laneEnds := []time.Time{}
	lanes := []int{}
	for _, r := range active {
		lane := 0
		for lane < len(laneEnds) && laneEnds[lane].After(r.Start) {
			lane++
		}
		if lane == len(laneEnds) {
			laneEnds = append(laneEnds, r.End)
		} else {
			laneEnds[lane] = r.End
		}
		lanes = append(lanes, lane)
	}
	width := 100
	if len(laneEnds) > 0 {
		width = 100 / len(laneEnds)
	}
	for i, r := range active {
		a, z := r.Start, r.End
		if a.Before(day) {
			a = day
		}
		if z.After(nextDay) {
			z = nextDay
		}
		label := r.Start.In(loc).Format("15:04") + " — " + r.End.In(loc).Format("15:04")
		cal.Events = append(cal.Events, ownerCalendarEvent{ID: r.ID, Name: r.Name, Time: label, Status: r.Status, StatusLabel: bookingStatusLabel(r.Status), Top: pos(a), Height: pos(z) - pos(a), Left: lanes[i] * width, Width: width})
	}
	cal.Count = len(active)
	return cal
}

func ownerBookingViews(rows []store.BookingView, loc *time.Location, now time.Time) []ownerBookingView {
	result := []ownerBookingView{}
	for _, row := range rows {
		result = append(result, ownerBookingView{BookingView: row, Date: row.Start.In(loc).Format("02.01.2006"), Time: row.Start.In(loc).Format("15:04") + " — " + row.End.In(loc).Format("15:04"), Price: fmt.Sprintf("%d.%02d", row.PriceCents/100, row.PriceCents%100), CanReschedule: row.Status == "confirmed" && row.Start.After(now), CanCancel: row.Status == "confirmed", CanComplete: row.Status == "confirmed" && !row.End.After(now), CanNoShow: row.Status == "confirmed" && !row.Start.After(now), StatusLabel: bookingStatusLabel(row.Status)})
	}
	return result
}

func selectedBooking(raw string, rows []store.BookingView) int64 {
	id, _ := strconv.ParseInt(raw, 10, 64)
	for _, r := range rows {
		if r.ID == id {
			return id
		}
	}
	for _, r := range rows {
		if r.Status != "cancelled" && r.Status != "no_show" {
			return r.ID
		}
	}
	if len(rows) > 0 {
		return rows[0].ID
	}
	return 0
}
