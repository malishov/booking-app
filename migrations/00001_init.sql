-- +goose Up
-- +goose StatementBegin

-- btree_gist is required for the exclusion constraint that will prevent
-- double bookings in a later migration.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- A master owns a booking page and configures their services and schedule.
CREATE TABLE masters (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE masters;
DROP EXTENSION IF EXISTS btree_gist;
-- +goose StatementEnd
