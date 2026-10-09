package handlers

import (
	"github.com/muradalishov/booking-app/backend/internal/store"
	"time"
)

type calendarDay struct {
	Number                     int
	Date, Label                string
	Available, Selected, Today bool
}
type calendarMonth struct {
	Name     string
	Year     int
	Selected bool
	Days     []calendarDay
}

var calendarMonthNames = []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}

func bookingCalendar(settings scheduleSettings, loc *time.Location, duration int32, busy []store.BusyInterval, now time.Time, selected string) []calendarMonth {
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	last := today.AddDate(0, 0, bookingHorizon)
	months := []calendarMonth{}
	for first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc); !first.After(last); first = first.AddDate(0, 1, 0) {
		month := calendarMonth{Name: calendarMonthNames[int(first.Month())-1], Year: first.Year()}
		for i := 0; i < (int(first.Weekday())+6)%7; i++ {
			month.Days = append(month.Days, calendarDay{})
		}
		for day := first; day.Month() == first.Month(); day = day.AddDate(0, 0, 1) {
			date := day.Format("2006-01-02")
			dayBusy := []store.BusyInterval{}
			for _, interval := range busy {
				if interval.Start.Before(day.AddDate(0, 0, 1)) && interval.End.After(day) {
					dayBusy = append(dayBusy, interval)
				}
			}
			available := len(availableSlots(settings, date, loc, duration, dayBusy, now)) > 0
			cell := calendarDay{Number: day.Day(), Date: date, Label: day.Format("02.01.2006"), Available: available, Selected: date == selected, Today: day.Equal(today)}
			if cell.Selected {
				month.Selected = true
			}
			month.Days = append(month.Days, cell)
		}
		for len(month.Days)%7 != 0 {
			month.Days = append(month.Days, calendarDay{})
		}
		months = append(months, month)
	}
	return months
}
