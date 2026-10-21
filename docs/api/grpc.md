# Справочник gRPC и отображение HTTP

Источник структуры — [warmhouse.proto](proto/warmhouse.proto), package `warmhouse.v1`. Обязательные проверки и семантика — [contracts.md](contracts.md). Поля строковых перечислений proto принимают только значения соответствующих JSON Schema/комментариев, не произвольные строки. proto3 сам не проверяет обязательность: непустые ID, Timestamp, Value.oneof, Parameters, конфигурация правила и остальные обязательные поля проверяются сервисом.

## Методы и разрешённые отправители

| Метод | Caller и контекст | Результат / назначение |
| --- | --- | --- |
| Users.ValidateSession | API Gateway, служебный токен; cookie в запросе | UserContext для выбранного audience, либо ошибка сессии/CSRF |
| Users.ExchangeContext | Telemetry или Scenarios, служебный токен и полученный subject_token | Новый контекст для Devices, исходный audience должен соответствовать caller |
| Devices.GetDevice | API Gateway или Scenarios, пользовательский контекст Devices | Реестр и возможности; Scenarios читает обе стороны правила |
| Devices.ListPollConfigurations | Telemetry, только служебный токен | Действующие снимки measurement-каналов и параметры опроса |
| Devices.CheckAccess | Telemetry/Scenarios, пользовательский контекст Devices | Разрешение на конкретный device/action |
| Devices.CheckSavedOwnerAccess | Scenarios, только служебный токен | Проверка сохранённого владельца для фонового правила |
| Devices.SubmitManualCommand | API Gateway, пользовательский контекст Devices | Command + replayed; владелец из контекста |
| Devices.SubmitScenarioCommand | Scenarios relay, только служебный токен | Command + replayed; сохранённый owner проверяется Devices |
| Devices.GetCommand | API Gateway, пользовательский контекст Devices | Команда только указанного устройства |
| Telemetry.ListTelemetry | API Gateway, пользовательский контекст Telemetry | История; кэш либо Users.ExchangeContext → Devices.CheckAccess |
| Scenarios.CreateRule | API Gateway, пользовательский контекст Scenarios | Новое правило и версия 1 |
| Scenarios.GetRule | API Gateway, пользовательский контекст Scenarios | Правило владельца |
| Scenarios.UpdateRule | API Gateway, пользовательский контекст Scenarios | Полная конфигурация с expected_version |
| Scenarios.ListTriggers | API Gateway, пользовательский контекст Scenarios | История срабатываний правила владельца |
| IntegrationGateway.GetPollJob | Telemetry, только служебный токен | pending/running/completed, либо NOT_FOUND |
| IntegrationGateway.RepublishPollResult | Telemetry, только служебный токен | Прежний ID результата после commit намерения повторной публикации |

Каждому направлению — свой `authorization: Bearer <service-token>`. Пользовательский контекст передаётся отдельно в `x-user-context`. `x-request-id` переносит безопасный ID запроса; trace context служит наблюдаемости и не подтверждает права. Gateway выпускает/переносит request_id, но не копирует произвольные служебные заголовки браузера. В логах запрещены cookie, токены и credentials адаптеров.

Для CheckAccess/CheckSavedOwnerAccess достоверный отказ возвращает OK с allowed=false без valid_until; ошибки проверки — UNAVAILABLE. Потребитель переводит allowed=false в PERMISSION_DENIED своей операции. Для чтения метаданных обоих устройств правила Scenarios использует GetDevice с обмененным контекстом; при изменении владения Devices повторно проверяет права во время приёма команды.

## HTTP ↔ gRPC

Перед каждой строкой Gateway выполняет ValidateSession с audience целевого сервиса. POST/PUT передают require_csrf=true. В таблице пути без общего `/api/v1`.

| HTTP operationId | RPC | Преобразование |
| --- | --- | --- |
| getDevice | Devices.GetDevice | path device_id → request; Device → 200 |
| listTelemetry | Telemetry.ListTelemetry | path, фильтры и page; from/to → Timestamp; TelemetryPage → 200 |
| submitCommand | Devices.SubmitManualCommand | path + CommandInput; replayed=false → 202, true → 200; ответ — command без обёртки |
| getCommand | Devices.GetCommand | оба path ID; Command → 200 |
| createRule | Scenarios.CreateRule | JSON → RuleInput; Rule → 201 и Location |
| getRule | Scenarios.GetRule | path rule_id; Rule → 200 |
| updateRule | Scenarios.UpdateRule | expected_version отделяется от RuleInput; Rule → 200 |
| listTriggers | Scenarios.ListTriggers | path + page; TriggerPage → 200 |

Rule.configuration разворачивается на верхний уровень HTTP вместе с id, owner_user_id, version и состоянием. Value.number_value/boolean_value/text_value превращается в JSON number/boolean/string; `0`, `false`, `""` сохраняются. Parameters.arguments преобразуется поэлементно. Timestamp всегда выводится в UTC с Z, без потери поддерживаемой наносекундной точности. Counter → десятичная строка. Presence необязательных proto-полей сохраняется; значения по умолчанию обязательных Boolean и массивов выводятся явно. Пустой next_cursor убирается из HTTP.

ID в path не переопределяется телом. Поля user_id/source/acceptance_seq/result_version запрещены в CommandInput. HTTP ошибки имеют форму Error; grpc status отображается по contracts.md. Доменная rejected-команда при повторе Submit возвращает прежний FAILED_PRECONDITION, а не успешный SubmitCommandResponse; разрешённый GetCommand может вернуть её запись со status=rejected.

## Повторы, deadline и сверка

Все RPC unary с deadline от caller; сервер не должен начинать неограниченную фоновую работу из-за истечения клиентского deadline. Отмена запроса не доказывает откат уже выполненного commit. Повторяемые транспортные ошибки: UNAVAILABLE и DEADLINE_EXCEEDED; INTERNAL требует разбора, не бесконечного автоматического повтора. Между попытками — ограниченная экспоненциальная пауза с jitter и общий бюджет. Значения задаются конфигурацией, не примерами документации.

| Группа | Политика |
| --- | --- |
| Get/List/Check | Повтор в рамках deadline/budget, каждый раз проверяется доступ |
| ValidateSession/ExchangeContext | Повтор допустим; токен может быть новым, права не расширяются |
| SubmitManualCommand/SubmitScenarioCommand | Повтор только с неизменными command_id, payload и expires_at |
| CreateRule | Автоматический повтор запрещён: после неоднозначного сбоя возможно создание дубликата |
| UpdateRule | После неоднозначного сбоя GET и сравнение; устаревшая expected_version даёт ABORTED |
| RepublishPollResult | Повтор допустим: тот же event_id/снимок; дубликат публикации поглощается inbox |

GetPollJob.completed содержит result_event_id/outcome. pending/running не содержат результата. RepublishPollResult для существующего незавершённого задания возвращает FAILED_PRECONDITION; для неизвестного — NOT_FOUND. Это внутренние ответы, а не HTTP 404 пользователю. publication_scheduled=true означает запись намерения публикации, не подтверждение доставки. Сохранение результата и его повторная публикация доступны любому экземпляру логического шлюза через собственную схему PostgreSQL.

## Примеры запросов gRPC

Это формы входа для grpcurl, а не сервер, доступный в репозитории. Служебные и пользовательские токены подставляются отдельно и не сохраняются в примерах. В grpcurl используется штатное ProtoJSON lowerCamelCase; браузерный HTTP остаётся snake_case.

```json
{
  "deviceId": "dev-thermostat",
  "command": {
    "commandId": "cmd-manual-1",
    "capabilityId": "cap-heating",
    "parameters": {"operation": "set", "arguments": {"enabled": {"booleanValue": true}}},
    "expiresAt": "2026-10-21T10:00:30Z"
  }
}
```

SubmitManualCommand возвращает:

```json
{
  "command": {
    "commandId": "cmd-manual-1", "deviceId": "dev-thermostat", "capabilityId": "cap-heating",
    "userId": "user-1", "source": "manual",
    "parameters": {"operation": "set", "arguments": {"enabled": {"booleanValue": true}}},
    "expiresAt": "2026-10-21T10:00:30Z", "createdAt": "2026-10-21T10:00:05Z",
    "status": "accepted", "acceptanceSeq": "41"
  },
  "replayed": false
}
```

Сверка:

```json
{"pollJobId": "poll-1"}
```

GetPollJob:

```json
{"pollJobId": "poll-1", "state": "completed", "resultEventId": "evt-poll-result-1", "outcome": "succeeded"}
```

RepublishPollResult:

```json
{"pollJobId": "poll-1", "resultEventId": "evt-poll-result-1", "publicationScheduled": true}
```

Результат всё равно применяется через PollCompleted → inbox Telemetry. Шлюз не возвращает измерение для прямой записи истории из этого RPC.
