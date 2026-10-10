-- +goose Up
-- Visit contacts are historical facts, independent of the customer directory.
-- Previous names overwritten before this migration cannot be reconstructed.
ALTER TABLE bookings ADD COLUMN client_name TEXT;
ALTER TABLE bookings ADD COLUMN client_phone TEXT;

UPDATE bookings b
SET client_name=c.name, client_phone=COALESCE(c.phone,'')
FROM customers c WHERE c.id=b.customer_id;

ALTER TABLE bookings ALTER COLUMN client_name SET NOT NULL;
ALTER TABLE bookings ALTER COLUMN client_phone SET NOT NULL;

-- An unverified/shared phone is a contact channel, not a unique identity.
-- Keep the lookup index without automatically merging clients by phone.
DROP INDEX idx_customers_business_phone;
CREATE INDEX idx_customers_business_phone ON customers(business_id,phone)
WHERE phone IS NOT NULL;

-- +goose Down
-- Downgrading requires shared-phone customers to have been explicitly resolved.
-- Never delete or merge customer records automatically to satisfy uniqueness.
DROP INDEX idx_customers_business_phone;
CREATE UNIQUE INDEX idx_customers_business_phone ON customers(business_id,phone)
WHERE phone IS NOT NULL;
ALTER TABLE bookings DROP COLUMN client_phone;
ALTER TABLE bookings DROP COLUMN client_name;
