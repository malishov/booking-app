# Frontend

Localized HTML pages, shared Go template components, local CSS and small
progressive JavaScript enhancements. No separate frontend server or build step
is required. `assets.go` embeds these files into the backend executable.

- `templates/shared.html`: common head, branding, icons and auth illustration.
- `templates/login.html`, `register.html`: master authentication forms.
- `templates/cabinet.html`: authenticated profile editor, public link and next steps.
- `templates/schedule.html`: weekly hours, daily breaks and extra days off.
- `templates/services.html`: service catalog with create, edit, hide and restore.
- `templates/online.html`: live public booking flow and confirmation.
- `templates/bookings.html`: private appointment list for the master.
- `templates/not-found.html`: missing page.
- `templates/business.html`: earlier profile-only template.
- `static/style.css`: shared styles and responsive layouts.
- `static/app.js`: password toggle, copy link and initial timezone suggestion.
- `locales/en.json`, `az.json`: English and Azerbaijani catalogs; Russian is the source language.

The language selector saves a visitor preference in a cookie for one year.
Templates are localized on the server, so language selection works without
JavaScript. The brand stays Окно in all three languages; user profile content is
never automatically translated.

Forms submit directly to backend routes and remain usable without JavaScript.
The example booking card is illustrative. Active services and prices appear on
the public master page; weekly schedules determine public slots; confirmed bookings appear in the private appointment list.
The appointments screen opens a day calendar with a week strip, work hours,
breaks and selectable client details. Upcoming visits and history remain lists.
On mobile, selecting a visit scrolls to its details below the timeline. Owners can
cancel visits, mark completed visits or no-shows, and book clients manually
through the same calendar and free-slot picker. All management forms work
without JavaScript; changes require the owner's authenticated session.
Future confirmed visits offer a reschedule action. Its calendar and slot picker
use the existing duration and price, show the current appointment details, and
return to the new day with the moved visit selected. RU, EN and AZ are supported.

Run from the repository root: `docker compose up --build -d`. Frontend files are
embedded at build time, so rebuild after changing them. Go's module and shared
Docker/sqlc configuration remain at the repository root; application logic and
database code are under `backend/`.
