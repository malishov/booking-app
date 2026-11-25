-- +goose Up
ALTER TABLE services ADD COLUMN price_unspecified boolean NOT NULL DEFAULT false;
ALTER TABLE services ADD CONSTRAINT service_unspecified_price_zero CHECK (NOT price_unspecified OR price_cents = 0);
ALTER TABLE booking_items ADD COLUMN price_unspecified boolean NOT NULL DEFAULT false;
ALTER TABLE booking_items ADD CONSTRAINT item_unspecified_price_zero CHECK (NOT price_unspecified OR price_cents = 0);

-- +goose Down
ALTER TABLE booking_items DROP COLUMN price_unspecified;
ALTER TABLE services DROP COLUMN price_unspecified;
