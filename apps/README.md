# Smart Home Sensor Management API

## Prerequisites

- Docker Engine and Docker Compose v2 with `up --wait` support
- Bash and curl for `init.sh`

## Getting Started

From `apps`, create the local configuration only if it does not already exist:

```bash
cd apps # from the repository root
if [ ! -e .env ]; then cp .env.example .env; fi
```

`.env` is ignored by Git; `.env.example` contains training values only. Compose
reads `.env` for substitution; the file is not sourced by Bash or copied into images.

| Variable | Example default | Purpose |
| --- | --- | --- |
| `APP_PORT` | `8080` | Published port of the monolith |
| `TEMPERATURE_API_PORT` | `8081` | Published port of the emulator |
| `POSTGRES_PASSWORD` | `postgres` | Required, nonempty database password; use training credentials only |

If port 8080 is occupied, set `APP_PORT=18080` in `.env`. Changing published ports
does not change internal addresses. PostgreSQL is not published on the host.
The monolith uses `PGPASSWORD` separately from its password-free database URI,
so URI characters in the password do not require URL encoding. In `.env`, single-quote
values containing literal `$` or `#` where necessary; do not execute the file as shell code.

Start all three services:

```bash
./init.sh
```

The script also works when invoked by its path from another working directory.
It builds the two applications, starts PostgreSQL, waits for its
TCP schema check and the emulator's health check, then checks the monolith's `/health`.
PostgreSQL uses the prebuilt official image. Alternatively:

```bash
docker compose up --build --wait --wait-timeout 120
```

The monolith is available at `http://localhost:<APP_PORT>` and the emulator at
`http://localhost:<TEMPERATURE_API_PORT>`. Defaults are 8080 and 8081 respectively.
Within the Compose network, the emulator is always `http://temperature-api:8081`;
future project services can use the same address when attached to that network.

```bash
docker compose ps
docker compose logs -f
docker compose stop
docker compose up -d --wait --wait-timeout 120
```

The existing fixed container names `smarthome-app` and `smarthome-postgres` must
be free before starting. When managing a stack started with `COMPOSE_PROJECT_NAME`,
use the same project name for every command. The recorded local verification stack
is `warmhouse-task5-20261022`; for example,
`COMPOSE_PROJECT_NAME=warmhouse-task5-20261022 docker compose ps`.
A different `COMPOSE_PROJECT_NAME` isolates the named volume
and network but does not isolate these names. Do not replace another project's containers.

## Data and emulator lifecycle

PostgreSQL stores data in the named `postgres_data` volume. `smart_home/init.sql`
creates `smarthome` and the sensor table only when the data directory is empty;
`POSTGRES_DB` is deliberately `postgres`. Restarting PostgreSQL preserves sensors.
Changing `POSTGRES_PASSWORD` does not change the password of an already initialized
database. Do not delete volumes to apply configuration changes automatically.

The emulator has no database or device registration. It maps `1` to `Living Room`,
`2` to `Bedroom`, and `3` to `Kitchen`. Unknown IDs retain their ID and return
`Unknown`; other nonempty locations retain their name and return sensor ID `0`.
It returns values from 15 to 30 °C in steps of 0.1, with no adjacent repeat for the
same sensor. Known IDs and their locations share history. History resets when the
emulator restarts. Concurrent response delivery order may differ from generation order.

## API Testing

Import [the Postman collection](smarthome-api.postman_collection.json) and set its
`baseUrl` variable to `http://localhost:18080` if using the local port override;
the collection default is `http://localhost:8080`.

1. Run `Create Sensor` with the existing request body and record the returned ID.
   Creation returns the database's initial value (0, status `inactive`); it does not
   fetch a measurement.
2. Run `Get All Sensors` repeatedly. The same temperature sensor must have a fresh
   `last_updated`, status `active`, and a different value on each consecutive read.
3. For `Get Sensor by ID`, substitute the actual created ID in the request path.

Checking HTTP 200 alone is insufficient: the monolith logs emulator errors and
may return stored values. Measurements fetched on reads update the response, not
stored history in the database.

Direct emulator checks (adjust published port if needed):

```bash
curl --fail 'http://localhost:8081/temperature?location=Living%20Room'
curl --fail http://localhost:8081/temperature/1
curl --fail http://localhost:8081/health
```

Both temperature routes return `value`, `unit`, UTC RFC3339 `timestamp`, `location`,
`status`, string `sensor_id`, `sensor_type`, and `description`. An empty or whitespace-only
location returns 400, an unknown route 404, and an unsupported method on a known
route 405. Errors contain a JSON `error` field. `/health` does not generate a reading.

## Local checks

```bash
(cd temperature-api && go test -race ./... && go vet ./...)
(cd smart_home && go test ./... && go vet ./...)
bash -n init.sh
docker compose config --quiet
docker compose --env-file .env.example config --quiet
```

The monolith currently has no unit tests. Compose validation does not prove runtime
readiness. Actual verification results are recorded in the root README, assignment 5.

## API Endpoints

- `GET /health` - Health check
- `GET /api/v1/sensors` - Get all sensors
- `GET /api/v1/sensors/:id` - Get a specific sensor
- `GET /api/v1/sensors/temperature/:location` - Temperature by location
- `POST /api/v1/sensors` - Create a new sensor
- `PUT /api/v1/sensors/:id` - Update a sensor
- `DELETE /api/v1/sensors/:id` - Delete a sensor
- `PATCH /api/v1/sensors/:id/value` - Update a sensor's value and status
