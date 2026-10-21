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

## Локальная основа MVP (этап 1)

[Демонстрационный профиль и границы](mvp-profile.md),
[генерация Go/JVM](contracts/README.md). Compose v2.24.4+ необходим для `!override`;
локально используется v2.29.1. Это отдельная конфигурация, не override legacy-файла.
Существующий стек задания 5 и его том сохраняются.

Из `apps`:

```bash
export COMPOSE_PROJECT_NAME=warmhouse-stage1-20261022
export COMPOSE_FILE=docker-compose.mvp.yml
# Для нового изолированного запуска выберите другое уникальное имя проекта.
docker compose --env-file .env.example config --quiet
docker compose --env-file .env.example build app temperature-api e2e contracts-go
docker compose --env-file .env.example up -d --wait --wait-timeout 180
docker compose --env-file .env.example --profile test run e2e npm run test:legacy
docker compose --env-file .env.example run --no-deps contracts-go
docker compose --env-file .env.example run --no-deps contracts-jvm
```

API: `http://127.0.0.1:18081`, порт настраивается `MVP_APP_PORT`.
Симулятор, БД и RabbitMQ не публикуются на host. `MVP_SUBNET` по умолчанию
`10.246.6.0/24`: перед новым запуском проверьте отсутствие пересечения с Docker/VPN;
если подсеть занята, укажите свободную. Это обходит исчерпанный автоматический
пул Docker без удаления сетей. Фиксированных container_name в MVP нет.
В `internal` используется учебная пара `mvp` / `mvp-local`, без TLS.
`POSTGRES_PASSWORD` обязателен; `.env.example` содержит только учебное значение.

Runner создаёт датчик с уникальным именем, выполняет Create → пять List → Get
и проверяет активный статус, диапазон/шаг, новые значения и свежие времена;
проверяет оба HTTP-маршрута симулятора и реальное AMQP-соединение.
Запросы ограничены 5 секундами, тесты — конечным deadline.
Удаление созданного датчика и cleanup не выполняются. Каждый `run` оставляет
контейнер для диагностики; `--rm` не используется. Для просмотра:

```bash
docker compose --env-file .env.example ps -a
docker compose --env-file .env.example logs --tail 100 app temperature-api rabbitmq
```

Этот раздел фиксирует основу этапа 1; актуальная общая приёмка приведена ниже.
Readiness четырёх процессов и legacy smoke не подтверждают новые микросервисы.

## Телеметрия MVP (этап 2)

Реализован один прибор `dev-thermostat` / `house-home` / `user-1`.
[Devices](devices/src/main/kotlin/warmhouse/devices/Application.kt) — Kotlin 2.1.10 /
Spring Boot 3.4.13, Gradle wrapper 8.12.1 / JDK 21; registry в своей схеме `devices`.
[Telemetry](telemetry/main.go) — Go/gRPC-Go, история в схеме `telemetry` с отдельной
ролью. Времена событий хранятся в каноническом UTC формате с девятью дробными
цифрами, чтобы сохранить наносекунды. Порядок этого формата хронологический.
[Симулятор](temperature-api/amqp.go) сохраняет старый переменный HTTP и в том же
процессе отвечает на AMQP опросы фиксированным `AMQP_TEMPERATURE` (17 по умолчанию).

Новые маршруты монолита:

- `GET /api/v1/devices/dev-thermostat` → Devices.GetDevice.
- `GET /api/v1/devices/dev-thermostat/telemetry` → Telemetry.ListTelemetry.

Монолит использует generated gRPC clients, не читает новые таблицы. Поддержаны
`capability_id`, `from` включительно, `to` исключительно, `limit` (1–500, default 100)
и подписанный keyset `cursor`. HTTP сохраняет snake_case, числовой скаляр (включая 0),
optional presence и UTC timestamps. GET читает сохранённые данные.

Свежий запуск из `apps` (имя, порт и подсеть должны быть свободны):

```bash
export COMPOSE_FILE=docker-compose.mvp.yml
export COMPOSE_PROJECT_NAME=warmhouse-stage2-20261022
export MVP_APP_PORT=18082
export MVP_SUBNET=10.246.7.0/24
export AMQP_TEMPERATURE=17
export SCHEDULER_ENABLED=true
docker compose --env-file .env.example config --quiet
docker compose --env-file .env.example build app devices telemetry temperature-api e2e contracts-go
docker compose --env-file .env.example up -d --wait --wait-timeout 120
docker compose --env-file .env.example run --no-deps e2e npm run test:telemetry
docker compose --env-file .env.example run --no-deps e2e npm run test:messages
docker compose --env-file .env.example run --no-deps e2e npm run test:legacy
# Host-generated bindings нужны для mounted native test runner:
docker compose --env-file .env.example run --no-deps contracts-go
docker compose --env-file .env.example run --no-deps grpc-tests
docker compose --env-file .env.example run --no-deps devices-tests ./gradlew --no-daemon --max-workers=2 --rerun-tasks test
```

Dockerfiles Go-приложений сами генерируют Go bindings из общего proto с закреплёнными
protoc/plugins; локальные generated-файлы исключены из build context. Build context
монолита и симулятора в legacy Compose теперь также корень репозитория; `init.sh`
сохраняет возможность сборки без host codegen. JVM contracts — общий Gradle subproject.
Порты gRPC 9090/9091, PostgreSQL, RabbitMQ и симулятора остаются внутри Compose-сети.
`up --wait` для процессов без healthcheck подтверждает только running; готовность
новых RPC подтверждают отдельные реальные boundary tests выше.

Для `0` использован отдельный свежий проект `warmhouse-stage2-zero-20261022`,
`MVP_APP_PORT=18083`, `MVP_SUBNET=10.246.8.0/24`, `AMQP_TEMPERATURE=0`.
Те же тесты получают expected value из runner env, а Go boundary test отдельно
проверяет присутствующий `Value.number_value=0`. Старый HTTP симулятора при этом
продолжает менять температуру. Не меняйте значения на старом томе ради проверки
нового seed и не удаляйте тома; выбирайте новое имя проекта.

Проверка отсутствия опроса из GET выполняется **на уже заполненной истории**:

```bash
SCHEDULER_ENABLED=false docker compose --env-file .env.example up -d --no-deps telemetry
SCHEDULER_ENABLED=false docker compose --env-file .env.example run --no-deps e2e npm run test:telemetry:readonly
SCHEDULER_ENABLED=true docker compose --env-file .env.example up -d --no-deps telemetry
```

Проверяйте фактический env telemetry через `docker inspect`: scheduler должен быть
выключен, а gRPC чтение доступно. Observer подписывается до GET, использует свою
очередь и не конкурирует с приложением. Проверка делает 42 GET и наблюдает ещё 5 s,
более двух периодов при стандартном `POLL_INTERVAL_MS=2000`.
Фоновое накопление проверяется отдельным интервалом 6,5 s без HTTP; тесты этого
профиля рассчитаны на стандартные interval=2000 / TTL=1000 ms. В Devices эти параметры
и `CONFIG_VALIDITY_MS` (default 300000) конфигурируемы. Не увеличивайте их при запуске
этих проверок без соответствующего изменения тестового временного бюджета.

Native Go tests/vet (на запущенном Compose, через `grpc-tests` с теми же mounts/env):

```bash
docker compose --env-file .env.example run --no-deps grpc-tests sh -c 'go test -race -count=1 -v ./... && go vet ./... && cd /workspace/apps/temperature-api && go test -race -count=1 ./... && go vet ./... && cd /workspace/apps/smart_home && go test ./... && go vet ./... && cd /workspace/apps/contracts/go && go test ./... && go vet ./...'
```

Для отдельно воспроизводимого существующего simulator test suite из repo root:
`docker build -f apps/temperature-api/Dockerfile.mvp -t warmhouse-stage2-simulator-tests .`,
затем `docker run warmhouse-stage2-simulator-tests`. Native Telemetry tests требуют
работающего настоящего сервиса; отсутствие стека — ошибка, а не skip.

Ограничения: [MVP profile](mvp-profile.md). Нет commands, outbox/inbox, надёжного
recovery, auth/TLS и физического оборудования. Поток успешного опроса не доказывает
доставку при сбоях; рестарт с выключенным scheduler здесь служит только проверкой
чтения сохранённой истории. Этапы 3–4 ещё не выполнялись.


## Ручные команды MVP: этап 3

Реализованы `POST /api/v1/devices/{device_id}/commands` и
`GET /api/v1/devices/{device_id}/commands/{command_id}` в монолите через
SubmitManualCommand/GetCommand Devices. Body POST — CommandInput из
[контрактов](../docs/api/grpc.md), ответ — Command без обёртки:
новый запрос получает 202, Location и accepted; GET достигает succeeded только
после результата операции симулятора. Поля user_id/source в body запрещены;
Devices задаёт демонстрационного пользователя и manual. Это не авторизация.

Проверенный отдельный проект этапа 3 использует порт 18084 и подсеть 10.246.9.0/24;
прежние стеки остаются нетронутыми. Команды из `apps`:

```bash
export MVP_APP_PORT=18084 MVP_SUBNET=10.246.9.0/24
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 build devices app temperature-api telemetry e2e grpc-tests
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 up -d --no-build --wait --wait-timeout 60
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 run --no-deps e2e npm run test:commands
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 run --no-deps e2e npm run test:messages
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 run --no-deps devices-tests ./gradlew --no-daemon --max-workers=2 --rerun-tasks test
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 run --no-deps grpc-tests
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 run --no-deps e2e npm run test:telemetry
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage3-20261022 run --no-deps e2e npm run test:legacy
```

Перед повторным запуском убедитесь, что порт/подсеть принадлежат именно вашему
проекту. Тесты команд запускать последовательно: они управляют одним каналом.
`test:commands` проверяет включение и отдельное выключение с новым command_id,
202/Location, succeeded и внутренний diagnostic enabled/last_command_id.
`test:messages` валидирует реальные CommandRequested/CommandStateChanged и
сообщения телеметрии по действующим JSON Schema с date-time и связями ID.
Observer подписан до отправки и использует свою очередь. JVM-тест дополнительно
проверяет присутствие Boolean oneof для true/false и optional counters.

Сохранённые значения `false`, sequence, cancel boundary, сроки/config и result_version
проверены на реальных границах. HTTP timestamps сохраняют ns; counters — строки.
Начальный HTTP RED был 404 вместо 202; тот же тест стал GREEN.
Ограничения доставки и симулятора перечислены в [MVP-профиле](mvp-profile.md).
Нет outbox/inbox/recovery/Scenarios/auth/TLS. Диагностический API не опубликован
на host и не является Devices API. На момент этапа 3 общая приёмка ещё не выполнялась;
её актуальные результаты приведены ниже.

## Общая приёмка MVP (этап 4)

Проверено 2026-10-22 на свежем проекте и новых PostgreSQL/RabbitMQ volumes.
Предыдущие разделы — история отдельных этапов. Для полного текущего запуска
используйте этот раздел. Требуются Docker Compose v2.24.4+ (проверено v2.29.1),
свободные порт/подсеть и доступ к registry/dependencies при первой сборке.
Не переиспользуйте старый project/volume как доказательство свежего запуска.

Фактически выполнено из `apps` (учебные credentials из `.env.example`):

```bash
export COMPOSE_FILE=docker-compose.mvp.yml
export COMPOSE_PROJECT_NAME=warmhouse-stage4-20261022
export MVP_APP_PORT=18085
export MVP_SUBNET=10.246.10.0/24
export AMQP_TEMPERATURE=0
export SCHEDULER_ENABLED=true
docker compose --env-file .env.example config --quiet
docker compose --env-file .env.example --profile test build app devices telemetry temperature-api e2e grpc-tests
docker compose --env-file .env.example up -d --no-build --wait --wait-timeout 180
docker compose --env-file .env.example run --no-deps e2e npm run test:all
```

Имя выше уже занято сохранённой приёмкой: для нового запуска выберите новое
уникальное имя и свободные порт/подсеть. `test:all` последовательно выполняет
legacy, telemetry, commands, messages — **PASS 3/3 + 5/5 + 2/2 + 2/2**, без skip.
Readiness RPC подтверждается реальными вызовами тестов; `up --wait` для Devices
и Telemetry без healthcheck подтверждает только running.

Seed создаётся повторяемо: `dev-thermostat` / `house-home` / `user-1`, температура
`cap-temperature` / Cel и отопление `cap-heating` / set enabled. AMQP=0 проверяет
сохранение нуля; старые HTTP readings остаются переменными 15–30 °C. Legacy suite
создаёт уникальный датчик и проверяет Create/List/Get. Остальной старый CRUD
сохранён в коде, но Update/Delete/Patch этим suite не покрываются.
Каждая команда получает новый UUID и будущий expires_at; observer готов до действия.
Проверяются реальные AMQP payload по действующим JSON Schema, версии, сроки,
config/sequence, связанные ID и диагностические enabled/last_command_id.
Ни CRUD, ни история старого монолита не перенесены в новые сервисы.

API доступен только на `http://127.0.0.1:18085`; gRPC, БД, RabbitMQ и diagnostic
симулятора остаются внутри сети. Демонстрационный пользователь фиксирован;
полноценной аутентификации, авторизации/Users, TLS и recovery нет.
Симулятор заменяет шлюз и прибор, хранит отопление в памяти. Нет outbox/inbox,
атомарности PostgreSQL+AMQP, обработки повторной доставки или exactly-once.
Риск буквального требования ООП-языков при выбранном Go сохраняется.

Логи и осмотр (те же env/project):

```bash
docker compose --env-file .env.example ps -a
docker compose --env-file .env.example logs --no-color --tail 100 app devices telemetry temperature-api
```

Логи содержат request_id, poll_job_id, command_id и event_id по соответствующему
потоку. Runner печатает ID, снимки команд и конечное состояние; ожидания ограничены.
Все one-off контейнеры и volumes оставлены, автоматической очистки нет.

Дополнительные проверки из `apps` с теми же env/project:

```bash
docker compose --env-file .env.example run --no-deps grpc-tests sh -c 'cd /workspace/apps/contracts/go && sh ../generate-go.sh && cd /workspace/apps/telemetry && go test -race -count=1 -v ./... && go vet ./... && cd /workspace/apps/temperature-api && go test -race -count=1 ./... && go vet ./... && cd /workspace/apps/smart_home && go test ./... && go vet ./...'
docker compose --env-file .env.example run --no-deps devices-tests ./gradlew --no-daemon --max-workers=2 --rerun-tasks test
SCHEDULER_ENABLED=false docker compose --env-file .env.example up -d --no-deps --no-build telemetry
docker inspect warmhouse-stage4-20261022-telemetry-1 --format '{{json .Config.Env}}'
SCHEDULER_ENABLED=false docker compose --env-file .env.example run --no-deps e2e npm run test:telemetry:readonly
SCHEDULER_ENABLED=true docker compose --env-file .env.example up -d --no-deps --no-build --wait --wait-timeout 60 telemetry
```

PASS: Go/JVM generation/compile; Go public ListTelemetry (`number_value=0`),
Devices public RPC tests 2/2 (`BOOLEAN_VALUE` true/false); существующие simulator
race tests и Go vet. У contracts/монолита предметных unit-тестов нет; JVM contracts
`test NO-SOURCE` не считается RPC-тестом. Readonly PASS: фактический env scheduler=false,
42 GET, сохранённая история неизменна и observer не получил PollRequested за 5 s.
После проверки scheduler возвращён в true. Это не тест восстановления после сбоя.

Из корня (host Node/npm и protoc нужны только для этих статических проверок):

```bash
task6_tmp=$(mktemp -d /tmp/warmhouse-contract-check-XXXXXX)
npm --prefix docs/api run check
protoc -I docs/api/proto --include_imports --descriptor_set_out="$task6_tmp/contracts.pb" docs/api/proto/warmhouse.proto
git diff --check
```

API check — PASS: OpenAPI, четыре AsyncAPI, 127 examples и cross-transport
invariants; descriptor — PASS. Спецификации этапом 4 не менялись, сборка их
просмотра и browser QA не повторялись. Это отдельные статические проверки,
не доказательство runtime. Recovery/security/нагрузка/оборудование — NOT RUN.
