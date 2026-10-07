-- name: GetBusinessByOwner :one
SELECT * FROM businesses WHERE owner_user_id = $1;

-- name: GetBusinessBySlug :one
SELECT * FROM businesses WHERE slug = $1;

-- name: SaveBusiness :one
INSERT INTO businesses (owner_user_id, name, slug, description, timezone)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (owner_user_id) DO UPDATE
SET name = EXCLUDED.name, slug = EXCLUDED.slug,
    description = EXCLUDED.description, timezone = EXCLUDED.timezone
RETURNING *;
