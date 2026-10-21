CREATE TABLE IF NOT EXISTS registry (id text PRIMARY KEY, house_id text NOT NULL, owner_id text NOT NULL, type_id text NOT NULL, name text NOT NULL);
INSERT INTO registry VALUES ('dev-thermostat','house-home','user-1','type-thermostat','Термостат гостиной') ON CONFLICT (id) DO NOTHING;

CREATE SEQUENCE IF NOT EXISTS command_acceptance_seq;
CREATE TABLE IF NOT EXISTS commands (
 command_id text PRIMARY KEY, device_id text NOT NULL REFERENCES registry(id),
 input text NOT NULL, snapshot text NOT NULL, acceptance_seq bigint NOT NULL,
 manual_cancel_before_seq bigint NOT NULL, expires_at text NOT NULL, config text NOT NULL
);
