package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	msg "warmhouse/contracts/messaging"
)

type heatingDiagnostic struct {
	Enabled       bool   `json:"enabled"`
	LastCommandID string `json:"last_command_id"`
}

var heating struct {
	sync.Mutex
	state heatingDiagnostic
}

func heatingState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	heating.Lock()
	state := heating.state
	heating.Unlock()
	writeJSON(w, 200, state)
}

func runCommands(ctx context.Context) error {
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
	deliveries, err := ch.Consume("wh.command.requests", "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	publisher, err := msg.NewPublisher(conn)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("command channel closed")
			}
			var p msg.CommandRequested
			if err = json.Unmarshal(d.Body, &p); err != nil {
				return err
			}
			enabled, present := p.Parameters.Arguments["enabled"].(bool)
			seq, seqErr := strconv.ParseUint(p.AcceptanceSeq, 10, 64)
			now := time.Now().UTC()
			c := p.Config
			if p.Type != "command.requested" || p.SchemaVersion != 1 || p.EventID == "" || p.CommandID == "" || p.UserID == "" || p.Source != "manual" || p.DeviceID != "dev-thermostat" || p.CapabilityID != "cap-heating" || c.DeviceID != p.DeviceID || c.CapabilityID != p.CapabilityID || c.ConfigVersion != 1 || c.AdapterKey != "partner-thermostat" || c.EndpointKey != "registered-endpoint-1" || c.Channel != "heating" || c.IssuedAt.After(now) || !now.Before(p.ExpiresAt) || p.ExpiresAt.After(c.ValidUntil) || seqErr != nil || seq == 0 || p.ManualCancelBeforeSeq != p.AcceptanceSeq || p.Parameters.Operation != "set" || !present || len(p.Parameters.Arguments) != 1 {
				// Invalid payload is a hard failure in this happy-path demo, not an acknowledged operation.
				return fmt.Errorf("invalid command payload command_id=%s", p.CommandID)
			}
			heating.Lock()
			heating.state = heatingDiagnostic{Enabled: enabled, LastCommandID: p.CommandID}
			completed := time.Now().UTC()
			heating.Unlock()
			result := msg.CommandStateChanged{CommandEnvelope: msg.CommandEnvelope{EventID: msg.ID(), Type: "command.state_changed", SchemaVersion: 1, OccurredAt: time.Now().UTC(), CommandID: p.CommandID, DeviceID: p.DeviceID, CapabilityID: p.CapabilityID}, ResultVersion: "1", Status: "succeeded", StateChangedAt: completed, ReasonCode: "COMPLETED", Evidence: "operation_completed"}
			call, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = publisher.Publish(call, "wh.events.v1", result.Type, result)
			cancel()
			if err != nil {
				return err
			}
			if err = d.Ack(false); err != nil {
				return err
			}
			log.Printf("command completed command_id=%s event_id=%s enabled=%t", p.CommandID, result.EventID, enabled)
		}
	}
}
