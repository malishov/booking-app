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
