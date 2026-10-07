-- +goose Up
ALTER TABLE services ADD COLUMN currency TEXT NOT NULL DEFAULT 'AZN'
    CHECK (currency IN ('AZN', 'AED', 'RUB', 'USD', 'EUR', 'GBP'));

-- +goose Down
ALTER TABLE services DROP COLUMN currency;
