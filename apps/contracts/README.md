# Контракты Go/JVM

Единственный исходник: [warmhouse.proto](../../docs/api/proto/warmhouse.proto).
Java protobuf и gRPC stubs доступны Kotlin/JVM без ручных DTO. Это генерация
контрактов всех RPC, а не реализация серверов. Сервис Devices и его Gradle wrapper
относятся к этапу 2; для текущей компиляции используется закреплённый Gradle image.

Из `apps` (Compose v2.24.4+):

```bash
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage1-20261022 build contracts-go
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage1-20261022 run --no-deps contracts-go
docker compose --env-file .env.example -f docker-compose.mvp.yml -p warmhouse-stage1-20261022 run --no-deps contracts-jvm
```

Go: protoc 3.21.12, protoc-gen-go 1.36.5, protoc-gen-go-grpc 1.5.1,
protobuf runtime 1.36.5, grpc-go 1.70.0. JVM: Gradle 8.12.1/JDK 21,
protobuf Gradle plugin 0.9.4, protoc/runtime 3.25.5, grpc-java 1.70.0.
Версии компиляторов языков могут различаться, номера/смысл полей едины.
Генерация Go также создаёт descriptor set и выполняет native `go test`/`go vet`;
Gradle выполняет `generateProto compileJava test`. Предметных RPC-тестов пока нет.

Выходы ignored: `go/gen`, `jvm/build`, `jvm/.gradle`. `go/go.sum` и Gradle
lockfile фиксируют зависимости. После осознанного изменения версий:
`contracts-jvm gradle --no-daemon generateProto compileJava test --write-locks`.
`LOCAL_UID`/`LOCAL_GID` задают владельца JVM-выходов на Linux.
Go-контейнер пишет generated-файлы как root; для чтения это не мешает.
Go-плагины установлены в `/usr/local/bin`, вне тома `/go`: пересборка образа
применяет закреплённые версии даже при сохранении существующего кэша.
Никакой cleanup командой генерации не выполняется.

Официальные инструкции: [Go codegen](https://grpc.io/docs/languages/go/quickstart/),
[JVM plugin и dependencies](https://github.com/grpc/grpc-java/blob/v1.70.0/README.md).
