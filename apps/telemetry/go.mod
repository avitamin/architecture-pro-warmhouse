module warmhouse/telemetry

go 1.22

require (
	github.com/jackc/pgx/v5 v5.3.1
	github.com/rabbitmq/amqp091-go v1.10.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
	warmhouse/contracts v0.0.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.0 // indirect
	golang.org/x/crypto v0.30.0 // indirect
	golang.org/x/net v0.32.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241202173237-19429a94021a // indirect
)

replace warmhouse/contracts => ../contracts/go
