# booking-app

Online booking for independent professionals.

## Problem

A professional (hairdresser, barber, tattoo artist, tutor) manages appointments
by hand — in messengers and notes. It is time-consuming and error-prone: two
clients end up booked for the same slot.

## MVP

- A master creates a booking page and configures their services and schedule.
- A client books **without registration** — picks a service and a free time slot.
- The system **reliably prevents double bookings** — enforced at the database
  level, not just in application code.

## Tech stack

| Layer         | Choice                             |
| ------------- | ---------------------------------- |
| Backend       | Go, `net/http` + `chi`             |
| Database      | PostgreSQL                         |
| DB access     | `pgx` + `sqlc`                     |
| Migrations    | `goose` (embedded via `embed`)     |
| Frontend      | Go templates + HTMX                |
| Tests         | `testing` + Testcontainers         |
| Runtime       | Docker Compose                     |
| CI            | GitHub Actions                     |

## Domain model

```mermaid
erDiagram
    USERS ||--o{ BUSINESSES : owns
    USERS ||--o| EMPLOYEES : "logs in as"
    BUSINESSES ||--o{ EMPLOYEES : employs
    BUSINESSES ||--o{ SERVICES : offers
    BUSINESSES ||--o{ CUSTOMERS : has
    EMPLOYEES ||--o{ WORKING_HOURS : "works"
    EMPLOYEES ||--o{ TIME_OFF : "is away"
    EMPLOYEES ||--o{ EMPLOYEE_SERVICES : provides
    SERVICES ||--o{ EMPLOYEE_SERVICES : "provided by"
    EMPLOYEES ||--o{ BOOKINGS : serves
    CUSTOMERS ||--o{ BOOKINGS : books
    BOOKINGS ||--o{ BOOKING_ITEMS : contains
    SERVICES ||--o{ BOOKING_ITEMS : "snapshotted in"

    USERS {
        bigint id PK
        text email UK
    }
    BUSINESSES {
        bigint id PK
        bigint owner_user_id FK
        text slug UK
        text timezone
    }
    EMPLOYEES {
        bigint id PK
        bigint business_id FK
        bigint user_id FK "nullable"
    }
    SERVICES {
        bigint id PK
        bigint business_id FK
        int duration_minutes
        int price_cents
    }
    EMPLOYEE_SERVICES {
        bigint employee_id FK
        bigint service_id FK
        int price_cents_override "nullable"
        int duration_minutes_override "nullable"
    }
    WORKING_HOURS {
        bigint employee_id FK
        smallint weekday "0=Mon..6=Sun"
        time start_time
        time end_time
    }
    TIME_OFF {
        bigint employee_id FK
        timestamptz starts_at
        timestamptz ends_at
    }
    CUSTOMERS {
        bigint id PK
        bigint business_id FK
        text phone
    }
    BOOKINGS {
        bigint id PK
        bigint employee_id FK
        bigint customer_id FK
        text client_name "snapshot"
        text client_phone "snapshot"
        timestamptz starts_at
        timestamptz ends_at
        text status
    }
    BOOKING_ITEMS {
        bigint id PK
        bigint booking_id FK
        bigint service_id FK "nullable"
        text service_name "snapshot"
        int price_cents "snapshot"
    }
```

Two design choices keep the model flexible without over-engineering:

- **`employee_services`** (many-to-many) records *which* employee performs *which*
  service, with optional per-employee price/duration overrides.
- **`booking_items`** lets one visit contain several services and **snapshots**
  the name, price, and duration at booking time, so later price changes never
  rewrite historical visits.

Double booking is prevented by a PostgreSQL exclusion constraint on `bookings`:
an employee cannot hold two active (non-cancelled) bookings whose time ranges
overlap. The guarantee lives in the database, so concurrent requests cannot race
past it.

## Layout

```
backend/
  cmd/server/    HTTP server entrypoint
  internal/
    auth/        Password and session token helpers
    config/      Configuration from environment variables
    db/          pgx pool and migration runner
    http/        Router, middleware, handlers
    store/       sqlc queries and generated data access
  migrations/    Embedded goose SQL migrations
frontend/
  templates/     HTML pages and shared template components
  static/        CSS, JavaScript and favicon
  assets.go      Embeds the frontend into the Go binary
```

## Quick start

```bash
docker compose up --build
```

Check it:

```bash
curl -i localhost:8080/health
# HTTP/1.1 200 OK
# {"status":"ok"}
```

`/health` pings the database and returns `503` if PostgreSQL is unreachable.

## Running locally (without Docker)

Requires a running PostgreSQL instance.

```bash
export DATABASE_URL="postgres://booking:booking@localhost:5432/booking?sslmode=disable"
export HTTP_ADDR=":8080"   # optional, defaults to :8080
export SESSION_COOKIE_SECURE=false # local HTTP only

go run ./backend/cmd/server
```

Migrations are applied automatically on startup.

## Configuration

| Variable       | Default   | Description                     |
| -------------- | --------- | ------------------------------- |
| `HTTP_ADDR`    | `:8080`   | Address the HTTP server binds   |
| `DATABASE_URL` | *(none)*  | PostgreSQL connection string    |
| `SESSION_COOKIE_SECURE` | `true` | HTTPS-only session cookie; set `false` for local HTTP |

## Master authentication

Register at `/register`, then log in at `/login`. Successful login creates a
7-day database session and redirects to `/cabinet`. The cabinet requires an
unexpired session. Use its logout button (`POST /logout`) to revoke the session.
Clients booking an appointment will not need accounts.

## Master page

Create or edit the master profile in `/cabinet`: name, optional description,
unique slug and IANA timezone. Each account owns one business page for this MVP.
The public profile at `/b/{slug}` is accessible without login and exposes no
owner account information. Changing the slug makes the old URL unavailable.
Manage services at `/cabinet/services`: name, duration (1–1440 minutes), price
and currency (AZN, AED, RUB, USD, EUR or GBP). Prices are stored in integer cents.
Services can be edited, hidden and restored. Only active services appear on the
public page; all private queries enforce the current business owner.
Schedules and online booking are available. Calendar selection for extra days
off, appointment cancellation and notifications are the next development steps.

The interface supports Russian, English and Azerbaijani. Language selection
persists in a cookie; the Окно brand and user-supplied content are not translated.

Profile saves always use the authenticated owner's ID. Slug uniqueness and the
one-business-per-owner rule are enforced by PostgreSQL. Timezone rules are
embedded into the binary for the minimal Docker runtime.

To run database integration tests, set `TEST_DATABASE_URL` to a development
PostgreSQL instance and run `go test ./...`. The test applies migrations inside
an isolated schema and transaction, which is rolled back on completion.

Session cookies are HttpOnly, SameSite=Lax, and Secure by default. The database
stores only SHA-256 hashes of cryptographically random 32-byte tokens. Logging in
again replaces and revokes the previous cookie's session. Cross-origin unsafe
requests are rejected using Go's `http.CrossOriginProtection`.

Docker Compose disables Secure cookies for local HTTP. Keep
`SESSION_COOKIE_SECURE=true` behind HTTPS in production. Expired sessions cannot
authenticate; old rows can be removed with
`DELETE FROM sessions WHERE expires_at <= now();`.

## Development

```bash
go test ./...        # tests
golangci-lint run    # linter
go build ./...       # build
```


### Master schedule

`/cabinet/schedule` configures weekly hours (Monday–Sunday), one optional
break per day, and up to 100 date-specific days off. New schedules start closed.
Times and dates use the business time zone. Overnight shifts are not supported.
Date-specific days off can be toggled in a monthly calendar and removed from the
selected date list. Changes apply after saving the schedule; existing bookings
are not cancelled. Without JavaScript, dates can still be entered as text.
Migration 00005 stores business-level settings in `master_schedules`; owner-scoped
queries replace the schedule atomically. Employee-level `working_hours` and
`time_off` remain reserved for future multi-employee support.
Client slot calculation and booking are implemented as described below.


### Online booking

Visitors choose an active service and date at `/b/{slug}`, then a free start time,
name and phone. No client account is required. Times use the business time zone.
Slots are spaced 15 minutes from shift start, within a 60-day horizon; the full
service must fit around breaks, days off and existing visits. Overnight shifts
are not supported. A public page starts closed until the owner saves working hours.
The public calendar marks only dates with at least one suitable slot as available.
Changing the service recalculates date availability. Initial visits select the
first available day; explicit date choices are preserved. With JavaScript off,
all supported months and the service refresh button remain usable.

`POST /b/{slug}/book` locks the business and rechecks the active service,
schedule and occupied times in a transaction. The existing PostgreSQL exclusion
constraint prevents overlapping active visits. A canonical solo employee is
created automatically. Client, visit and price/name/duration/currency snapshots
commit together. Migration 00006 adds the solo employee index, currency snapshot
and opaque receipt hash.

Migration 00007 snapshots the client's name and phone on each visit. Cabinet
lists, calendar details and rescheduling read these snapshots rather than the
customer directory. A public or manual booking creates a separate customer
record; an unverified/shared phone never automatically merges people or renames
an existing client. Explicit owner-driven customer merging is future work.
Existing visits are backfilled from the current directory; names overwritten
before this migration cannot be reconstructed. Downgrading to phone uniqueness
requires shared-phone customer records to be explicitly resolved first.

Migration 00008 reserves current and future former public addresses in
`business_slug_aliases`. Old `/b/{slug}` pages redirect to the current address,
preserving service/date selection; old booking forms can still submit to their
original business. Only that business can reuse a former address. Redirects
are temporary and not cached, avoiding loops when an owner restores an address.
Public addresses discarded before the migration cannot be reconstructed.

New client confirmations use `/booked/{token}`, independent of the public
address. Legacy `/b/{slug}/booked/{token}` links resolve the hashed token and
redirect to the same permanent receipt, even if their old slug was discarded
before the migration. Invalid tokens return 404. Confirmations and redirects
use `no-store` and `no-referrer`; both receipt route formats redact tokens in
request logs. Confirmation pages still omit client names and phone numbers.

Success redirects to an unguessable confirmation URL. This page exposes only
appointment details, not client name or phone. The owner manages appointments
at `/cabinet/bookings`; other owners cannot access them.

### Managing visits

The owner cabinet opens a daily calendar in the business's local time zone,
with date selection, week navigation, work hours, breaks and selectable visit
details. Cancelled visits and no-shows remain available in the day's list without
occupying the timeline. The daily calendar includes every visit overlapping the
selected day. Upcoming (active visits, soonest first) and History (past visits
and terminal statuses) each display up to 200 visits.
Owners can cancel confirmed visits, mark
them completed after their end, or mark a no-show after their start. Terminal
statuses cannot be changed again. Cancellation/no-show releases occupancy;
completed visits retain their original time range and price snapshot.

`POST /cabinet/bookings/{id}/status` scopes changes to the session owner and
shares the business transaction lock with booking creation. The client receipt
shows the current status. The owner must inform the client about a cancellation.

`/cabinet/bookings/new` creates a confirmed visit for an active service using the
same calendar, availability checks and transaction as public booking. Client
accounts are not required; manual booking cannot bypass hours, days off, breaks,
the 60-day horizon or existing visits. No notification is sent automatically.
`/cabinet/bookings/{id}/reschedule` lets the owner move a future confirmed visit
to an available date and time within the same 60-day horizon. Availability uses
the original duration snapshot and excludes the visit's own occupancy, allowing
partially overlapping moves. The current exact start is omitted from choices.
The transaction shares the business lock with creation and status changes,
checks the submitted original start to reject stale forms, and keeps the client,
service/price/duration/currency snapshots and receipt URL. The receipt shows the
new time. Past or terminal visits cannot be moved. Owners notify clients manually.

Notifications, client self-cancellation and multi-employee booking remain future
work.
