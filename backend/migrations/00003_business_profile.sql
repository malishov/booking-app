-- +goose Up
ALTER TABLE businesses ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE businesses ADD CONSTRAINT businesses_owner_user_id_key UNIQUE (owner_user_id);

-- +goose Down
ALTER TABLE businesses DROP CONSTRAINT businesses_owner_user_id_key;
ALTER TABLE businesses DROP COLUMN description;
