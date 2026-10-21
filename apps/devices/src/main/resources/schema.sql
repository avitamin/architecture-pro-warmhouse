CREATE TABLE IF NOT EXISTS registry (id text PRIMARY KEY, house_id text NOT NULL, owner_id text NOT NULL, type_id text NOT NULL, name text NOT NULL);
INSERT INTO registry VALUES ('dev-thermostat','house-home','user-1','type-thermostat','Термостат гостиной') ON CONFLICT (id) DO NOTHING;
