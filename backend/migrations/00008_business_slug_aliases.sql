-- +goose Up
-- Keep concurrent profile writes out until current addresses are reserved and
-- the trigger is installed in this same migration transaction.
LOCK TABLE businesses IN SHARE ROW EXCLUSIVE MODE;
CREATE TABLE business_slug_aliases (
    slug TEXT PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses(id) ON DELETE CASCADE
);
CREATE INDEX business_slug_aliases_business ON business_slug_aliases(business_id);
INSERT INTO business_slug_aliases(slug,business_id) SELECT slug,id FROM businesses;

-- Reserve current and previous addresses in a single unique namespace.
-- +goose StatementBegin
CREATE FUNCTION reserve_business_slug() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO business_slug_aliases(slug,business_id) VALUES(NEW.slug,NEW.id)
    ON CONFLICT(slug) DO UPDATE SET business_id=EXCLUDED.business_id
    WHERE business_slug_aliases.business_id=EXCLUDED.business_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'business address already reserved'
        USING ERRCODE='23505', CONSTRAINT='businesses_slug_key';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER business_slug_reservation AFTER INSERT OR UPDATE OF slug ON businesses
FOR EACH ROW EXECUTE FUNCTION reserve_business_slug();

-- +goose Down
DROP TRIGGER business_slug_reservation ON businesses;
DROP FUNCTION reserve_business_slug();
DROP TABLE business_slug_aliases;
