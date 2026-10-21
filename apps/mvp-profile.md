# Локальный демонстрационный MVP — этапы 1–2

Это учебный профиль на одном компьютере, не production-развёртывание.
Запуск описан в [apps README](README.md#локальная-основа-mvp-этап-1).
Целевые [HTTP/gRPC контракты](../docs/api/README.md) сохраняются.

| Граница | Реализуется в MVP | Текущее состояние / отложено |
| --- | --- | --- |
| Legacy HTTP | Create/List/Get и остальные существующие `/api/v1/sensors` | Сохранён Go-монолит и его БД; ответ чтения получает меняющуюся температуру |
| Новый HTTP | GET device, GET telemetry | Реализованы в монолите через generated gRPC clients |
| Новый HTTP | POST command, GET command | Этап 3, маршруты пока отсутствуют |
| Devices RPC | GetDevice, ListPollConfigurations | Kotlin/Spring Boot, seed в схеме devices |
| Devices RPC | SubmitManualCommand, GetCommand | Этап 3 |
| Telemetry RPC | ListTelemetry и независимый scheduler | Go/gRPC-Go, history в схеме telemetry, опрос независимо от GET |
| Прочие RPC | Users, Scenarios, access checks, IntegrationGateway recovery | Отложены; фиктивные успешные ответы не реализуются |
| AMQP infrastructure | RabbitMQ, vhost `internal`, соединение runner | Этап 1; порты брокера не опубликованы |
| AMQP telemetry | poll.requested → measurement.recorded + poll.completed | Реальные durable topic exchanges/очереди, JSON Schema и связанные event_id |
| AMQP commands | command.requested → command.state_changed | Этап 3; обработчики пока не создаются |
| Единый симулятор | Существующий `temperature-api`, совместимый HTTP | HTTP и telemetry AMQP в одном процессе; команды отложены |
| Температура | HTTP 15–30 °C, шаг 0,1, без соседнего повтора | `AMQP_TEMPERATURE` задаёт фиксированное AMQP-значение; 17 по умолчанию, 0 поддержан отдельно |
| Пользователь и seed | user-1, house-home, dev-thermostat, cap-temperature/Cel, cap-heating/set/enabled | Повторяемый seed одного прибора и фиксированного владельца user-1 без Users |
| Генерация | Общий proto → Go и JVM protobuf/gRPC | Этап 1, native compile/check без предметных сервисов |
| Тестовая граница | node:test, Ajv 2020 + date-time, AMQP client, реальный legacy HTTP | Legacy, telemetry HTTP/AMQP и native public RPC tests; commands/test:all ещё отсутствуют |
| Сохранность | Именованные project-scoped volumes PostgreSQL/RabbitMQ | Не сбрасывать старые тома ради init.sql; новая проверка использует новый project |
| Изоляция и безопасность | Только HTTP монолита на `127.0.0.1`; остальные порты внутри сети | Демонстрационные credentials; нет production auth, TLS, cookie/CSRF и прав Users |
| Доставка и восстановление | Успешный поток с точными контрактами | Нет outbox/inbox, exactly-once, атомарности БД+AMQP, lease, failover/recovery |
| Оборудование | Симулятор вместо шлюза/прибора | Эффект в симуляторе не доказывает физическое исполнение; нет Elixir/MQTT |

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

Команды, сквозной false, физическое исполнение, recovery и production security —
**NOT RUN** / вне этапа 2. Генерация всех RPC не означает их поддержку;
нереализованные RPC отвечают UNIMPLEMENTED. Риск буквального требования
ООП-языков при выбранном Go остаётся.
