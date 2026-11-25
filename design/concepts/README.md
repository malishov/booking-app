# Варианты управления записями

Интерактивные концепции для сравнения в чате, с демо-данными:

- `okno-compact-list.html`: компактный список и детали выбранного клиента.
- `okno-day-calendar.html`: временная шкала рабочего дня и детали встречи.
- `okno-next-client.html`: главный акцент на ближайшем клиенте.

Это HTML-фрагменты для визуального сравнения. Действия локальные и не меняют
записи приложения. Перенос визита здесь показан как предлагаемая функция.
Основной интерфейс кабинета остаётся в `frontend/templates/bookings.html`.

## Design checkpoint — 2026-11-25

Latest concept: `okno-crafted.html` (desktop and mobile). Open this file through a local HTTP server so that `font-study-assets/` resolves correctly.

Direction: Schedulo-inspired booking structure, Atelier-inspired confirmation, with the independent practitioner as the main face of the page. The selected font is Inter; local font files and their OFL licenses are included. The palette uses soft sage, warm off-white and white.

The latest concept includes a demo portrait, a three-step flow (service → date/time → contacts), list-based service selection and preview controls for long service names, missing prices, unavailable dates and phone validation. Preview controls are not part of the client interface. The portrait is illustrative; replace the demo name “Лейла Алиева” later as requested.

`okno-fonts.html` preserves the type comparison. Earlier concepts are kept for comparison.

The latest visual concept is not yet transferred into the application templates. The working application already contains the earlier booking redesign, separate service/time steps, calendar and compact slot selection, confirmation actions and optional service prices. Its implementation notes are in `../changes/2026-11-21/README.md`.

Next session: review the latest desktop/mobile concept, then transfer the approved changes into the application.
