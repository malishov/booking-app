-- +goose Up
-- +goose StatementBegin

-- btree_gist backs the exclusion constraint that prevents double bookings.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Account that can log in (business owner, and optionally staff).
CREATE TABLE users (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A booking page owned by a user; holds its own timezone.
CREATE TABLE businesses (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_user_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    slug          TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL,
    timezone      TEXT        NOT NULL DEFAULT 'UTC',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A person who performs services. May be linked to a login (solo owner) or not.
CREATE TABLE employees (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT      NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    user_id     BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    name        TEXT        NOT NULL,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_employees_business ON employees (business_id);

-- A service offered by a business, with base price and duration.
CREATE TABLE services (
    id               BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id      BIGINT      NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    name             TEXT        NOT NULL,
    duration_minutes INTEGER     NOT NULL CHECK (duration_minutes > 0),
    price_cents      INTEGER     NOT NULL CHECK (price_cents >= 0),
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_services_business ON services (business_id);

-- Which employee performs which service, with optional per-employee overrides.
-- (employee/service belonging to the same business is enforced in the app layer.)
CREATE TABLE employee_services (
    employee_id               BIGINT  NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    service_id                BIGINT  NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    price_cents_override      INTEGER CHECK (price_cents_override >= 0),
    duration_minutes_override INTEGER CHECK (duration_minutes_override > 0),
    PRIMARY KEY (employee_id, service_id)
);
CREATE INDEX idx_employee_services_service ON employee_services (service_id);

-- Recurring weekly availability. weekday: 0 = Monday ... 6 = Sunday.
-- Multiple rows per weekday are allowed (split shifts, e.g. morning + evening).
CREATE TABLE working_hours (
    id          BIGINT   GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id BIGINT   NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    weekday     SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time  TIME     NOT NULL,
    end_time    TIME     NOT NULL,
    CONSTRAINT working_hours_valid CHECK (end_time > start_time)
);
CREATE INDEX idx_working_hours_employee ON working_hours (employee_id);

-- One-off exceptions that block availability (vacation, sick day, break).
CREATE TABLE time_off (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id BIGINT      NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    starts_at   TIMESTAMPTZ NOT NULL,
    ends_at     TIMESTAMPTZ NOT NULL,
    reason      TEXT,
    CONSTRAINT time_off_valid CHECK (ends_at > starts_at)
);
CREATE INDEX idx_time_off_employee ON time_off (employee_id);

-- Clients are scoped per business and never register (identified by phone/email).
CREATE TABLE customers (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT      NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    phone       TEXT,
    email       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_customers_business ON customers (business_id);
-- Dedupe repeat clients by phone within a business.
CREATE UNIQUE INDEX idx_customers_business_phone
    ON customers (business_id, phone) WHERE phone IS NOT NULL;

-- A visit: a single time window served by one employee for one customer.
CREATE TABLE bookings (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT      NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    employee_id BIGINT      NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    customer_id BIGINT      NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    starts_at   TIMESTAMPTZ NOT NULL,
    ends_at     TIMESTAMPTZ NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'confirmed', 'cancelled', 'completed', 'no_show')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bookings_time_valid CHECK (ends_at > starts_at),
    -- Core guarantee: an employee cannot hold two active bookings that overlap.
    -- Enforced by the database, so concurrent requests cannot race past it.
    CONSTRAINT bookings_no_overlap EXCLUDE USING gist (
        employee_id WITH =,
        tstzrange(starts_at, ends_at) WITH &&
    ) WHERE (status NOT IN ('cancelled', 'no_show'))
);
CREATE INDEX idx_bookings_business_time ON bookings (business_id, starts_at);
CREATE INDEX idx_bookings_employee_time ON bookings (employee_id, starts_at);

-- Line items of a visit. Price/duration/name are snapshotted at booking time so
-- later edits to a service never rewrite historical visits.
CREATE TABLE booking_items (
    id               BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    booking_id       BIGINT  NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    service_id       BIGINT  REFERENCES services (id) ON DELETE SET NULL,
    service_name     TEXT    NOT NULL,
    price_cents      INTEGER NOT NULL CHECK (price_cents >= 0),
    duration_minutes INTEGER NOT NULL CHECK (duration_minutes > 0)
);
CREATE INDEX idx_booking_items_booking ON booking_items (booking_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS booking_items;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS time_off;
DROP TABLE IF EXISTS working_hours;
DROP TABLE IF EXISTS employee_services;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS employees;
DROP TABLE IF EXISTS businesses;
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS btree_gist;

-- +goose StatementEnd
