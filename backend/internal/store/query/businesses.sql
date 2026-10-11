-- name: GetBusinessByOwner :one
SELECT * FROM businesses WHERE owner_user_id = $1;

-- name: GetBusinessBySlug :one
SELECT b.* FROM businesses b JOIN business_slug_aliases a ON a.business_id=b.id WHERE a.slug = $1;

-- name: GetReceiptBusiness :one
SELECT p.* FROM businesses p JOIN bookings b ON b.business_id=p.id WHERE b.receipt_hash=$1;

-- name: SaveBusiness :one
INSERT INTO businesses (owner_user_id, name, slug, description, timezone)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (owner_user_id) DO UPDATE
SET name = EXCLUDED.name, slug = EXCLUDED.slug,
    description = EXCLUDED.description, timezone = EXCLUDED.timezone
RETURNING *;
