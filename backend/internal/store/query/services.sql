-- name: ListServicesByOwner :many
SELECT services.* FROM services
JOIN businesses ON businesses.id = services.business_id
WHERE businesses.owner_user_id = $1 ORDER BY services.id;

-- name: ListPublicServices :many
SELECT * FROM services WHERE business_id = $1 AND is_active = true ORDER BY id;

-- name: GetServiceByOwner :one
SELECT services.* FROM services
JOIN businesses ON businesses.id = services.business_id
WHERE services.id = $1 AND businesses.owner_user_id = $2;

-- name: CreateService :one
INSERT INTO services (business_id, name, duration_minutes, price_cents, currency)
SELECT id, $2, $3, $4, $5 FROM businesses WHERE owner_user_id = $1
RETURNING *;

-- name: UpdateService :one
UPDATE services SET name = $3, duration_minutes = $4, price_cents = $5, currency = $6
FROM businesses
WHERE services.business_id = businesses.id AND businesses.owner_user_id = $2 AND services.id = $1
RETURNING services.*;

-- name: ToggleService :one
UPDATE services SET is_active = NOT services.is_active
FROM businesses
WHERE services.business_id = businesses.id AND businesses.owner_user_id = $2 AND services.id = $1
RETURNING services.*;
