-- +goose Up
CREATE UNIQUE INDEX employees_solo_owner ON employees(business_id,user_id) WHERE user_id IS NOT NULL;
ALTER TABLE booking_items ADD COLUMN currency TEXT NOT NULL DEFAULT 'AZN';
ALTER TABLE bookings ADD COLUMN receipt_hash TEXT UNIQUE;

-- +goose Down
ALTER TABLE bookings DROP COLUMN receipt_hash;
ALTER TABLE booking_items DROP COLUMN currency;
DROP INDEX employees_solo_owner;
