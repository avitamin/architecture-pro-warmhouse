package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "warmhouse/contracts/gen/warmhouse/v1"
)

// These boundary tests require the real Compose Telemetry, not a local fake server.
func TestPublicListTelemetry(t *testing.T) {
	conn, err := grpc.NewClient(env("TELEMETRY_GRPC_ADDR", "telemetry:9091"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewTelemetryServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	capability := "cap-temperature"
	page, err := client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", CapabilityId: &capability, Page: &pb.PageRequest{Limit: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("need two saved records and next_cursor: %v", page)
	}
	expected, err := strconv.ParseFloat(env("EXPECTED_TEMPERATURE", "17"), 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range page.Items {
		if m.DeviceId != "dev-thermostat" || m.CapabilityId != capability || m.GetUnit() != "Cel" || m.Id == "" || m.EventId == "" {
			t.Fatalf("wrong metadata: %v", m)
		}
		value, ok := m.Value.Value.(*pb.Value_NumberValue)
		if !ok || value.NumberValue != expected {
			t.Fatalf("Value.oneof did not retain number %v: %v", expected, m.Value)
		}
		if m.MeasuredAt == nil || m.GatewayReceivedAt == nil || m.MeasuredAt.CheckValid() != nil || m.GatewayReceivedAt.CheckValid() != nil {
			t.Fatalf("missing timestamps: %v", m)
		}
	}
	second, err := client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", CapabilityId: &capability, Page: &pb.PageRequest{Limit: 2, Cursor: page.NextCursor}})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) == 0 || !second.Items[0].MeasuredAt.AsTime().Before(page.Items[1].MeasuredAt.AsTime()) {
		t.Fatalf("cursor must continue in descending order: %v", second)
	}
	m := page.Items[0]
	filtered, err := client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", CapabilityId: &capability, From: m.MeasuredAt, To: timestamppb.New(m.MeasuredAt.AsTime().Add(time.Microsecond))})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].Id != m.Id {
		t.Fatalf("inclusive from/exclusive to: %v", filtered)
	}
	excluded, err := client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", To: m.MeasuredAt, Page: &pb.PageRequest{Limit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(excluded.Items) != 1 || excluded.Items[0].Id == m.Id {
		t.Fatal("to must exclude exact boundary")
	}
	other := "cap-heating"
	empty, err := client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", CapabilityId: &other})
	if err != nil || len(empty.GetItems()) != 0 {
		t.Fatalf("capability filter ignored: %v %v", empty, err)
	}
	_, err = client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", CapabilityId: &other, Page: &pb.PageRequest{Cursor: page.NextCursor}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("cross-filter cursor: %v", err)
	}
	_, err = client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "dev-thermostat", Page: &pb.PageRequest{Cursor: page.NextCursor + "tampered"}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("tampered cursor: %v", err)
	}
	_, err = client.ListTelemetry(ctx, &pb.ListTelemetryRequest{DeviceId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("missing device: %v", err)
	}
	t.Logf("verified number=%s event_id=%s measured_at=%s", os.Getenv("EXPECTED_TEMPERATURE"), m.EventId, m.MeasuredAt.AsTime().Format(time.RFC3339Nano))
}
