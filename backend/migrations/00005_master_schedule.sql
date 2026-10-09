-- +goose Up
CREATE TABLE master_schedules (
    business_id BIGINT PRIMARY KEY REFERENCES businesses(id) ON DELETE CASCADE,
    settings JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE master_schedules;
