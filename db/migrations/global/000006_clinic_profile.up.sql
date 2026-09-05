-- Public clinic discovery: display metadata shown on patient-facing clinic
-- details (address, contact, opening hours). Kept in the global schema next
-- to tenants so clinic browsing never needs to probe tenant schemas.
ALTER TABLE tenants
    ADD COLUMN address     VARCHAR(255),
    ADD COLUMN city        VARCHAR(100),
    ADD COLUMN phone       VARCHAR(32),
    ADD COLUMN email       VARCHAR(255),
    ADD COLUMN description TEXT,
    ADD COLUMN hours       JSONB NOT NULL DEFAULT '{}';