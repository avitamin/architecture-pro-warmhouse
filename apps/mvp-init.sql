\connect smarthome
CREATE ROLE devices LOGIN PASSWORD 'devices-local';
CREATE ROLE telemetry LOGIN PASSWORD 'telemetry-local';
CREATE SCHEMA devices AUTHORIZATION devices;
CREATE SCHEMA telemetry AUTHORIZATION telemetry;
GRANT CONNECT ON DATABASE smarthome TO devices, telemetry;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
