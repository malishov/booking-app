package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/muradalishov/booking-app/backend/internal/store"
)

type ScheduleStore interface {
	GetBusinessByOwner(context.Context, int64) (store.Business, error)
	GetSchedule(context.Context, int64) ([]byte, error)
	SaveSchedule(context.Context, int64, []byte) error
}
type Schedule struct {
	store  ScheduleStore
	tmpl   *template.Template
	logger *slog.Logger
}

func NewSchedule(s ScheduleStore, t *template.Template, l *slog.Logger) *Schedule {
	return &Schedule{s, t, l}
}

type scheduleDay struct {
	Index                            int
	Name                             string
	Enabled                          bool
	Start, End, BreakStart, BreakEnd string
}
type scheduleSettings struct {
	Days    []scheduleDay
	DaysOff string
}
type schedulePage struct {
	Email    string
	Business store.Business
	scheduleSettings
	Error         string
	Saved         bool
	WorkingDays   int
	CalendarToday string
}

var dayNames = []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}

func defaultSchedule() scheduleSettings {
	s := scheduleSettings{}
	for i, name := range dayNames {
		s.Days = append(s.Days, scheduleDay{Index: i, Name: name, Start: "09:00", End: "18:00"})
	}
	return s
}
func (s *Schedule) profile(w http.ResponseWriter, r *http.Request) (store.Business, bool) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", 303)
		return store.Business{}, false
	}
	b, err := s.store.GetBusinessByOwner(r.Context(), u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Redirect(w, r, "/cabinet?setup=schedule#profile-editor", 303)
		return b, false
	}
	if err != nil {
		s.fail(w, err)
		return b, false
	}
	return b, true
}
func (s *Schedule) Page(w http.ResponseWriter, r *http.Request) {
	b, ok := s.profile(w, r)
	if !ok {
		return
	}
	raw, err := s.store.GetSchedule(r.Context(), b.OwnerUserID)
	if err != nil {
		s.fail(w, err)
		return
	}
	settings := defaultSchedule()
	if err := json.Unmarshal(raw, &settings); err != nil || len(settings.Days) != 7 {
		s.fail(w, errors.New("invalid stored schedule"))
		return
	}
	// Labels are presentation data; always derive them from the canonical weekdays.
	for i := range settings.Days {
		settings.Days[i].Index = i
		settings.Days[i].Name = dayNames[i]
	}
	s.render(w, r, b, settings, "", 200)
}
func (s *Schedule) Save(w http.ResponseWriter, r *http.Request) {
	b, ok := s.profile(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", 400)
		return
	}
	settings := defaultSchedule()
	for i := range settings.Days {
		p := "day_" + strconv.Itoa(i) + "_"
		d := &settings.Days[i]
		d.Enabled = r.PostFormValue(p+"enabled") == "on"
		d.Start = strings.TrimSpace(r.PostFormValue(p + "start"))
		d.End = strings.TrimSpace(r.PostFormValue(p + "end"))
		d.BreakStart = strings.TrimSpace(r.PostFormValue(p + "break_start"))
		d.BreakEnd = strings.TrimSpace(r.PostFormValue(p + "break_end"))
	}
	settings.DaysOff = strings.TrimSpace(r.PostFormValue("days_off"))
	if message := validateSchedule(&settings); message != "" {
		s.render(w, r, b, settings, message, 400)
		return
	}
	raw, err := json.Marshal(settings)
	if err == nil {
		err = s.store.SaveSchedule(r.Context(), b.OwnerUserID, raw)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/cabinet/schedule?saved=1", 303)
}
func validateSchedule(s *scheduleSettings) string {
	if len(s.Days) != 7 {
		return "Укажите расписание для семи дней."
	}
	for _, d := range s.Days {
		if !d.Enabled {
			continue
		}
		a, e1 := time.Parse("15:04", d.Start)
		b, e2 := time.Parse("15:04", d.End)
		if e1 != nil || e2 != nil || !a.Before(b) {
			return "Время окончания должно быть позже начала. Ночные смены пока не поддерживаются."
		}
		if d.BreakStart != "" || d.BreakEnd != "" {
			c, e3 := time.Parse("15:04", d.BreakStart)
			e, e4 := time.Parse("15:04", d.BreakEnd)
			if e3 != nil || e4 != nil || !a.Before(c) || !c.Before(e) || !e.Before(b) {
				return "Перерыв должен начинаться и заканчиваться внутри рабочего дня."
			}
		}
	}
	dates := []string{}
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(s.DaysOff, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		date := strings.TrimSpace(raw)
		if date == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return "Введите даты выходных в формате ГГГГ-ММ-ДД."
		}
		if !seen[date] {
			dates = append(dates, date)
			seen[date] = true
		}
	}
	if len(dates) > 100 {
		return "Можно указать не больше 100 отдельных выходных."
	}
	s.DaysOff = strings.Join(dates, "\n")
	return ""
}
func (s *Schedule) render(w http.ResponseWriter, r *http.Request, b store.Business, settings scheduleSettings, message string, status int) {
	u, _ := CurrentUser(r.Context())
	data := schedulePage{Email: u.Email, Business: b, scheduleSettings: settings, Error: message, Saved: r.URL.Query().Get("saved") == "1"}
	loc, err := time.LoadLocation(b.Timezone)
	if err != nil {
		s.fail(w, err)
		return
	}
	data.CalendarToday = time.Now().In(loc).Format("2006-01-02")
	for _, d := range settings.Days {
		if d.Enabled {
			data.WorkingDays++
		}
	}
	renderHTML(w, r, s.tmpl, s.logger, "schedule.html", data, status)
}
func (s *Schedule) fail(w http.ResponseWriter, err error) {
	s.logger.Error("schedule failed", "error", err)
	http.Error(w, "Internal server error", 500)
}
