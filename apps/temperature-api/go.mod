module temperature-api

go 1.22

require (
	github.com/rabbitmq/amqp091-go v1.10.0
	warmhouse/contracts v0.0.0
)

replace warmhouse/contracts => ../contracts/go
