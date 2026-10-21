package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	msg "warmhouse/contracts/messaging"
)

func runPolls(ctx context.Context) error {
	conn, err := amqp.Dial(os.Getenv("AMQP_URL"))
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err = msg.Topology(ch); err != nil {
		return err
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume("wh.poll.requests", "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	publisher, err := msg.NewPublisher(conn)
	if err != nil {
		return err
	}
	raw := os.Getenv("AMQP_TEMPERATURE")
	if raw == "" {
		raw = "17"
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("invalid AMQP_TEMPERATURE")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("poll channel closed")
			}
			var p msg.PollRequested
			if err = json.Unmarshal(d.Body, &p); err != nil {
				return err
			}
			c := p.Config
			if p.DeviceID != "dev-thermostat" || p.CapabilityID != "cap-temperature" || c.DeviceID != p.DeviceID || c.CapabilityID != p.CapabilityID || c.AdapterKey != "partner-thermostat" || c.EndpointKey != "registered-endpoint-1" || c.Channel != "temperature" || !time.Now().Before(p.ExpiresAt) || p.ExpiresAt.After(c.ValidUntil) {
				d.Nack(false, false)
				continue
			}
			measured := time.Now().UTC()
			received := time.Now().UTC()
			m := msg.MeasurementRecorded{Envelope: msg.Event("measurement.recorded", p.PollJobID, p.DeviceID, p.CapabilityID), Value: value, MeasuredAt: measured, GatewayReceivedAt: received, Unit: "Cel"}
			r := msg.PollCompleted{Envelope: msg.Event("poll.completed", p.PollJobID, p.DeviceID, p.CapabilityID), Outcome: "succeeded", MeasurementEventID: m.EventID}
			call, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = publisher.Publish(call, "wh.events.v1", m.Type, m)
			if err == nil {
				err = publisher.Publish(call, "wh.events.v1", r.Type, r)
			}
			cancel()
			if err != nil {
				return err
			}
			if err = d.Ack(false); err != nil {
				return err
			}
			log.Printf("poll completed poll_job_id=%s measurement_event_id=%s event_id=%s", p.PollJobID, m.EventID, r.EventID)
		}
	}
}
