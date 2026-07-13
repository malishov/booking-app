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
cmd/server/      Entrypoint: the HTTP server
internal/
  config/        Configuration from environment variables
  db/            pgx pool and migration runner
  http/          Router (chi), middleware, handlers
migrations/      goose SQL migrations (embedded via embed)
web/             Static assets and HTML templates
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

go run ./cmd/server
```

Migrations are applied automatically on startup.

## Configuration

| Variable       | Default   | Description                     |
| -------------- | --------- | ------------------------------- |
| `HTTP_ADDR`    | `:8080`   | Address the HTTP server binds   |
| `DATABASE_URL` | *(none)*  | PostgreSQL connection string    |

## Development

```bash
go test ./...        # tests
golangci-lint run    # linter
go build ./...       # build
```
