# Frontend

Russian-language HTML pages, shared Go template components, local CSS and small
progressive JavaScript enhancements. No separate frontend server or build step
is required. `assets.go` embeds these files into the backend executable.

- `templates/shared.html`: common head, branding, icons and auth illustration.
- `templates/login.html`, `register.html`: master authentication forms.
- `templates/cabinet.html`: authenticated profile editor, public link and next steps.
- `templates/services.html`: service catalog with create, edit, hide and restore.
- `templates/business.html`, `not-found.html`: public master profile and missing page.
- `static/style.css`: shared styles and responsive layouts.
- `static/app.js`: password toggle, copy link and initial timezone suggestion.
- `locales/en.json`, `az.json`: English and Azerbaijani catalogs; Russian is the source language.

The language selector saves a visitor preference in a cookie for one year.
Templates are localized on the server, so language selection works without
JavaScript. The brand stays Окно in all three languages; user profile content is
never automatically translated.

Forms submit directly to backend routes and remain usable without JavaScript.
The example booking card is illustrative. Active services and prices appear on
the public master page; schedules and booking are not implemented yet.

Run from the repository root: `docker compose up --build -d`. Frontend files are
embedded at build time, so rebuild after changing them. Go's module and shared
Docker/sqlc configuration remain at the repository root; application logic and
database code are under `backend/`.
