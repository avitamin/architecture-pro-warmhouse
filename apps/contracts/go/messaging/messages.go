// Package messaging contains the JSON transport types for the telemetry MVP.
package messaging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	DeviceID      string    `json:"device_id"`
	CapabilityID  string    `json:"capability_id"`
	ConfigVersion int32     `json:"config_version"`
	AdapterKey    string    `json:"adapter_key"`
	EndpointKey   string    `json:"endpoint_key"`
	Channel       string    `json:"channel"`
	IssuedAt      time.Time `json:"issued_at"`
	ValidUntil    time.Time `json:"valid_until"`
	ValueType     string    `json:"value_type,omitempty"`
	Unit          string    `json:"unit,omitempty"`
}
type Envelope struct {
	EventID       string    `json:"event_id"`
	Type          string    `json:"type"`
	SchemaVersion int       `json:"schema_version"`
	OccurredAt    time.Time `json:"occurred_at"`
	PollJobID     string    `json:"poll_job_id"`
	DeviceID      string    `json:"device_id"`
	CapabilityID  string    `json:"capability_id"`
}
type PollRequested struct {
	Envelope
	ExpiresAt time.Time `json:"expires_at"`
	Config    Config    `json:"config"`
}
type MeasurementRecorded struct {
	Envelope
	Value             float64   `json:"value"`
	MeasuredAt        time.Time `json:"measured_at"`
	GatewayReceivedAt time.Time `json:"gateway_received_at"`
	Unit              string    `json:"unit"`
}
type PollCompleted struct {
	Envelope
	Outcome            string `json:"outcome"`
	MeasurementEventID string `json:"measurement_event_id"`
}

func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Event(kind, job, device, capability string) Envelope {
	return Envelope{ID(), kind, 1, time.Now().UTC(), job, device, capability}
}
func Topology(ch *amqp.Channel) error {
	for _, ex := range []string{"wh.jobs.v1", "wh.events.v1"} {
		if err := ch.ExchangeDeclare(ex, "topic", true, false, false, false, nil); err != nil {
			return err
		}
	}
	for _, binding := range [][3]string{{"wh.poll.requests", "poll.requested", "wh.jobs.v1"}, {"wh.telemetry.measurements", "measurement.recorded", "wh.events.v1"}, {"wh.telemetry.poll-results", "poll.completed", "wh.events.v1"}} {
		if _, err := ch.QueueDeclare(binding[0], true, false, false, false, nil); err != nil {
			return err
		}
		if err := ch.QueueBind(binding[0], binding[1], binding[2], false, nil); err != nil {
			return err
		}
	}
	return nil
}

// Publisher owns its channel; callers serialize publications. Confirms establish broker acceptance only.
type Publisher struct {
	ch       *amqp.Channel
	returns  <-chan amqp.Return
	confirms <-chan amqp.Confirmation
}

func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err = ch.Confirm(false); err != nil {
		ch.Close()
		return nil, err
	}
	return &Publisher{ch, ch.NotifyReturn(make(chan amqp.Return, 1)), ch.NotifyPublish(make(chan amqp.Confirmation, 1))}, nil
}
func (p *Publisher) Publish(ctx context.Context, exchange, key string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err = p.ch.PublishWithContext(ctx, exchange, key, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body}); err != nil {
		return err
	}
	select {
	case c, ok := <-p.confirms:
		if !ok || !c.Ack {
			return fmt.Errorf("publication not confirmed")
		}
		select {
		case r := <-p.returns:
			return fmt.Errorf("unroutable: %s", r.ReplyText)
		default:
			return nil
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}
