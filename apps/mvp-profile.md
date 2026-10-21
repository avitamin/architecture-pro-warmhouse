# Локальный демонстрационный MVP — этап 1

Это учебный профиль на одном компьютере, не production-развёртывание.
Запуск описан в [apps README](README.md#локальная-основа-mvp-этап-1).
Целевые [HTTP/gRPC контракты](../docs/api/README.md) сохраняются.

| Граница | Реализуется в MVP | Текущее состояние / отложено |
| --- | --- | --- |
| Legacy HTTP | Create/List/Get и остальные существующие `/api/v1/sensors` | Сохранён Go-монолит и его БД; ответ чтения получает меняющуюся температуру |
| Новый HTTP | GET device, GET telemetry | Этап 2, маршруты пока отсутствуют |
| Новый HTTP | POST command, GET command | Этап 3, маршруты пока отсутствуют |
| Devices RPC | GetDevice, ListPollConfigurations | Этап 2; Kotlin/Spring Boot ещё не создавался |
| Devices RPC | SubmitManualCommand, GetCommand | Этап 3 |
| Telemetry RPC | ListTelemetry и независимый scheduler | Этап 2; Go-сервис ещё не создавался |
| Прочие RPC | Users, Scenarios, access checks, IntegrationGateway recovery | Отложены; фиктивные успешные ответы не реализуются |
| AMQP infrastructure | RabbitMQ, vhost `internal`, соединение runner | Этап 1; порты брокера не опубликованы |
| AMQP telemetry | poll.requested → measurement.recorded + poll.completed | Этап 2; exchanges/bindings и обработчики пока не создаются |
| AMQP commands | command.requested → command.state_changed | Этап 3; обработчики пока не создаются |
| Единый симулятор | Существующий `temperature-api`, совместимый HTTP | Сейчас только HTTP; AMQP добавляется в тот же процесс на этапах 2–3 |
| Температура | HTTP 15–30 °C, шаг 0,1, без соседнего повтора | `AMQP_TEMPERATURE=17` зарезервирован отдельно для этапа 2, сейчас не читается кодом |
| Пользователь и seed | user-1, house-home, dev-thermostat, cap-temperature/Cel, cap-heating/set/enabled | Этапы 2–3; фиксированный пользователь будет из конфигурации без Users |
| Генерация | Общий proto → Go и JVM protobuf/gRPC | Этап 1, native compile/check без предметных сервисов |
| Тестовая граница | node:test, Ajv 2020 + date-time, AMQP client, реальный legacy HTTP | Этап 1; будущие test:telemetry/test:commands/test:all добавляются вместе с тестами |
| Сохранность | Именованные project-scoped volumes PostgreSQL/RabbitMQ | Не сбрасывать старые тома ради init.sql; новая проверка использует новый project |
| Изоляция и безопасность | Только HTTP монолита на `127.0.0.1`; остальные порты внутри сети | Демонстрационные credentials; нет production auth, TLS, cookie/CSRF и прав Users |
| Доставка и восстановление | Успешный поток с точными контрактами | Нет outbox/inbox, exactly-once, атомарности БД+AMQP, lease, failover/recovery |
| Оборудование | Симулятор вместо шлюза/прибора | Эффект в симуляторе не доказывает физическое исполнение; нет Elixir/MQTT |

Новые потоки, сохранение 0/false между всеми границами, корреляция event_id,
исполнение команд, надёжность и безопасность пока **NOT RUN**. Генерация всех
RPC не означает их поддержку. Риск буквального требования ООП-языков при выбранном
Go остаётся; языки утверждены пользователем и автоматически не заменяются.
