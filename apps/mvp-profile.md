# Локальный демонстрационный MVP — этапы 1–3

Это учебный профиль на одном компьютере, не production-развёртывание.
Запуск описан в [apps README](README.md#локальная-основа-mvp-этап-1).
Целевые [HTTP/gRPC контракты](../docs/api/README.md) сохраняются.

| Граница | Реализуется в MVP | Текущее состояние / отложено |
| --- | --- | --- |
| Legacy HTTP | Create/List/Get и остальные существующие `/api/v1/sensors` | Сохранён Go-монолит и его БД; ответ чтения получает меняющуюся температуру |
| Новый HTTP | GET device, GET telemetry | Реализованы в монолите через generated gRPC clients |
| Новый HTTP | POST command, GET command | Реализованы в монолите через Devices gRPC; новый POST → 202/Location/accepted |
| Devices RPC | GetDevice, ListPollConfigurations | Kotlin/Spring Boot, seed в схеме devices |
| Devices RPC | SubmitManualCommand, GetCommand | Хранение команды, sequence/config/expiry в devices; результат применяется после события |
| Telemetry RPC | ListTelemetry и независимый scheduler | Go/gRPC-Go, history в схеме telemetry, опрос независимо от GET |
| Прочие RPC | Users, Scenarios, access checks, IntegrationGateway recovery | Отложены; фиктивные успешные ответы не реализуются |
| AMQP infrastructure | RabbitMQ, vhost `internal`, соединение runner | Этап 1; порты брокера не опубликованы |
| AMQP telemetry | poll.requested → measurement.recorded + poll.completed | Реальные durable topic exchanges/очереди, JSON Schema и связанные event_id |
| AMQP commands | command.requested → command.state_changed | Прямая persistent/mandatory публикация с confirm; succeeded только после операции симулятора |
| Единый симулятор | Существующий `temperature-api`, совместимый HTTP | HTTP, telemetry и command AMQP в одном процессе; отопление в памяти |
| Температура | HTTP 15–30 °C, шаг 0,1, без соседнего повтора | `AMQP_TEMPERATURE` задаёт фиксированное AMQP-значение; 17 по умолчанию, 0 поддержан отдельно |
| Пользователь и seed | user-1, house-home, dev-thermostat, cap-temperature/Cel, cap-heating/set/enabled | Повторяемый seed одного прибора и фиксированного владельца user-1 без Users |
| Генерация | Общий proto → Go и JVM protobuf/gRPC | Этап 1, native compile/check без предметных сервисов |
| Тестовая граница | node:test, Ajv 2020 + date-time, AMQP client, реальный legacy HTTP | Legacy, telemetry HTTP/AMQP и native public RPC tests; commands и message tests; test:all относится к этапу 4 |
| Сохранность | Именованные project-scoped volumes PostgreSQL/RabbitMQ | Не сбрасывать старые тома ради init.sql; новая проверка использует новый project |
| Изоляция и безопасность | Только HTTP монолита на `127.0.0.1`; остальные порты внутри сети | Демонстрационные credentials; нет production auth, TLS, cookie/CSRF и прав Users |
| Доставка и восстановление | Успешный поток с точными контрактами | Нет outbox/inbox, exactly-once, атомарности БД+AMQP, lease, failover/recovery |
| Оборудование | Симулятор вместо шлюза/прибора | Эффект в симуляторе не доказывает физическое исполнение; нет Elixir/MQTT |

Проверки этапа 3: [ручные команды](README.md#ручные-команды-mvp-этап-3).
Проверки этапа 2 перечислены в [apps README](README.md#телеметрия-mvp-этап-2).
История хранит исходные UTC timestamps с наносекундной точностью. Фильтры
capability_id/from/to и keyset cursor поддержаны; cursor подписан и привязан к
ресурсу/фильтрам единственного demo-пользователя. Это не проверка прав Users.

Фоновый процесс держит один текущий poll_job_id, ожидает измерение и связанный
результат либо истечение TTL. Scheduler можно явно выключить конфигурацией
`SCHEDULER_ENABLED=false`, сохранив чтение уже записанной истории. Постоянные
job records, inbox, повторная доставка и восстановление после restart отсутствуют.
ACK измерения следует после commit; публикации persistent, mandatory и с confirm.
Это не атомарность БД+AMQP и не доказательство обработки получателем.

Этап 3 проверяет включение и отдельное выключение с новым ID: HTTP → generated
protobuf → реальный AMQP → операция симулятора → событие результата → Devices → HTTP.
`false` сохраняется как Boolean oneof и JSON Boolean. Внутренний GET
`http://temperature-api:8081/diagnostics/heating` возвращает `enabled` и
`last_command_id`; при legacy-запуске без AMQP_URL этот маршрут отсутствует.
Host-порт симулятора в MVP не публикуется. `user_id` берётся из `DEMO_USER_ID`
Devices (по умолчанию user-1), source=manual задаётся сервером.

Новая команда сначала сохраняется как accepted без result_version, затем напрямую
публикуется. Снимок первого успешного Submit остаётся accepted даже при быстром
результате; GetCommand показывает актуальный снимок. acceptance_seq назначается
Devices, manual_cancel_before_seq равен ему; expires_at/config сохраняются.
Успешный результат симулятора имеет result_version=1, evidence=operation_completed,
reason_code=COMPLETED. Publisher confirm не переводит команду в succeeded.
Срок новой команды должен быть в будущем и помещаться в CONFIG_VALIDITY_MS
(по умолчанию 300000 ms); это ограниченный демонстрационный профиль.

Есть окно потери между БД и публикацией; повтор принятого ID возвращает сохранённый
снимок, но не восстанавливает потерянную публикацию. Симулятор не хранит команды
долговечно; нет обработки повторной доставки, приоритета сценариев или recovery.
Невалидная команда останавливает AMQP-процесс симулятора, ошибка результата
останавливает его consumer в Devices без ACK; quarantine/retry не реализованы.
Физическое исполнение, recovery, production security и этап 4 — **NOT RUN**. Генерация всех RPC не означает их поддержку;
нереализованные RPC отвечают UNIMPLEMENTED. Риск буквального требования
ООП-языков при выбранном Go остаётся.
