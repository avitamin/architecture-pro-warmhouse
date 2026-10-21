package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "warmhouse/contracts/gen/warmhouse/v1"
	msg "warmhouse/contracts/messaging"
)

type telemetry struct {
	pb.UnimplementedTelemetryServiceServer
	db        *pgxpool.Pool
	cursorKey []byte
}
type cursor struct{ Device, Capability, From, To, Time, ID string }

func (s *telemetry) encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(append(b, mac.Sum(nil)...))
}
func (s *telemetry) decodeCursor(v string) (cursor, error) {
	var c cursor
	b, e := base64.RawURLEncoding.DecodeString(v)
	if e != nil || len(b) <= 32 {
		return c, errors.New("invalid cursor")
	}
	payload, signature := b[:len(b)-32], b[len(b)-32:]
	m := hmac.New(sha256.New, s.cursorKey)
	m.Write(payload)
	if !hmac.Equal(signature, m.Sum(nil)) {
		return c, errors.New("invalid cursor")
	}
	e = json.Unmarshal(payload, &c)
	return c, e
}
func (s *telemetry) ListTelemetry(ctx context.Context, r *pb.ListTelemetryRequest) (*pb.TelemetryPage, error) {
	if r.DeviceId != "dev-thermostat" {
		return nil, status.Error(codes.NotFound, "unknown device")
	}
	limit := r.GetPage().GetLimit()
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		return nil, status.Error(codes.InvalidArgument, "limit must be 1..500")
	}
	var from, to any
	var fromKey, toKey string
	if r.From != nil {
		if e := r.From.CheckValid(); e != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid from")
		}
		from = storageTime(r.From.AsTime())
		fromKey = r.From.AsTime().Format(time.RFC3339Nano)
	}
	if r.To != nil {
		if e := r.To.CheckValid(); e != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid to")
		}
		to = storageTime(r.To.AsTime())
		toKey = r.To.AsTime().Format(time.RFC3339Nano)
	}
	if from != nil && to != nil && !r.From.AsTime().Before(r.To.AsTime()) {
		return nil, status.Error(codes.InvalidArgument, "from must precede to")
	}
	var lastTime, lastID any
	if r.GetPage().GetCursor() != "" {
		c, e := s.decodeCursor(r.Page.Cursor)
		if e != nil || c.Device != r.DeviceId || c.Capability != r.GetCapabilityId() || c.From != fromKey || c.To != toKey {
			return nil, status.Error(codes.InvalidArgument, "cursor does not match query")
		}
		t, e := time.Parse(time.RFC3339Nano, c.Time)
		if e != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor time")
		}
		lastTime = storageTime(t)
		lastID = c.ID
	}
	rows, e := s.db.Query(ctx, `SELECT id,event_id,device_id,capability_id,value,measured_at,gateway_received_at,unit FROM telemetry.measurements WHERE device_id=$1 AND ($2='' OR capability_id=$2) AND ($3::text IS NULL OR measured_at >= $3) AND ($4::text IS NULL OR measured_at < $4) AND ($5::text IS NULL OR (measured_at,id)<($5,$6::text)) ORDER BY measured_at DESC,id DESC LIMIT $7`, r.DeviceId, r.GetCapabilityId(), from, to, lastTime, lastID, int(limit)+1)
	if e != nil {
		return nil, status.Error(codes.Internal, "history query failed")
	}
	defer rows.Close()
	page := &pb.TelemetryPage{Items: []*pb.Measurement{}}
	for rows.Next() {
		m := &pb.Measurement{}
		var value float64
		var measuredText, receivedText string
		var unit string
		if e = rows.Scan(&m.Id, &m.EventId, &m.DeviceId, &m.CapabilityId, &value, &measuredText, &receivedText, &unit); e != nil {
			return nil, status.Error(codes.Internal, "history decode failed")
		}
		m.Value = &pb.Value{Value: &pb.Value_NumberValue{NumberValue: value}}
		measured, err := time.Parse(time.RFC3339Nano, measuredText)
		if err != nil {
			return nil, status.Error(codes.Internal, "invalid stored time")
		}
		received, err := time.Parse(time.RFC3339Nano, receivedText)
		if err != nil {
			return nil, status.Error(codes.Internal, "invalid stored time")
		}
		m.MeasuredAt = timestamppb.New(measured)
		m.GatewayReceivedAt = timestamppb.New(received)
		m.Unit = &unit
		page.Items = append(page.Items, m)
	}
	if e = rows.Err(); e != nil {
		return nil, status.Error(codes.Internal, "history read failed")
	}
	if len(page.Items) > int(limit) {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = s.encodeCursor(cursor{r.DeviceId, r.GetCapabilityId(), fromKey, toKey, last.MeasuredAt.AsTime().Format(time.RFC3339Nano), last.Id})
	}
	return page, nil
}

// Fixed-width UTC timestamps retain protobuf nanoseconds; their lexical order is chronological.
func storageTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

func env(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, e := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS telemetry.measurements(id text PRIMARY KEY,event_id text UNIQUE NOT NULL,device_id text NOT NULL,capability_id text NOT NULL,value double precision NOT NULL,measured_at text NOT NULL,gateway_received_at text NOT NULL,unit text NOT NULL);`)
	if e != nil {
		log.Fatal(e)
	}
	conn, e := amqp.Dial(os.Getenv("AMQP_URL"))
	if e != nil {
		log.Fatal(e)
	}
	defer conn.Close()
	ch, e := conn.Channel()
	if e != nil {
		log.Fatal(e)
	}
	defer ch.Close()
	if e = msg.Topology(ch); e != nil {
		log.Fatal(e)
	}
	measurements, e := ch.Consume("wh.telemetry.measurements", "", false, false, false, false, nil)
	if e != nil {
		log.Fatal(e)
	}
	results, e := ch.Consume("wh.telemetry.poll-results", "", false, false, false, false, nil)
	if e != nil {
		log.Fatal(e)
	}
	publisher, e := msg.NewPublisher(conn)
	if e != nil {
		log.Fatal(e)
	}
	deviceConn, e := grpc.NewClient(env("DEVICES_GRPC_ADDR", "devices:9090"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if e != nil {
		log.Fatal(e)
	}
	defer deviceConn.Close()
	devices := pb.NewDevicesServiceClient(deviceConn)
	service := &telemetry{db: db, cursorKey: []byte(env("CURSOR_KEY", "mvp-local-telemetry-cursor-key"))}
	server := grpc.NewServer()
	pb.RegisterTelemetryServiceServer(server, service)
	listener, e := net.Listen("tcp", ":9091")
	if e != nil {
		log.Fatal(e)
	}
	go func() {
		if e := server.Serve(listener); e != nil {
			log.Print(e)
			stop()
		}
	}()
	defer server.Stop()
	// One outstanding poll. A result only releases the job once its measurement is committed.
	go func() {
		enabled, e := strconv.ParseBool(env("SCHEDULER_ENABLED", "true"))
		if e != nil {
			log.Fatal(e)
		}
		var current string
		var measurementID, resultID string
		var expires, next time.Time
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-measurements:
				if !ok {
					stop()
					return
				}
				var m msg.MeasurementRecorded
				if e = json.Unmarshal(d.Body, &m); e != nil {
					log.Print(e)
					d.Nack(false, false)
					continue
				}
				if m.PollJobID != current || m.DeviceID != "dev-thermostat" || m.CapabilityID != "cap-temperature" {
					d.Nack(false, false)
					continue
				}
				_, e = db.Exec(ctx, `INSERT INTO telemetry.measurements VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, msg.ID(), m.EventID, m.DeviceID, m.CapabilityID, m.Value, storageTime(m.MeasuredAt), storageTime(m.GatewayReceivedAt), m.Unit)
				if e != nil {
					log.Print(e)
					stop()
					return
				}
				measurementID = m.EventID
				d.Ack(false)
				log.Printf("measurement committed poll_job_id=%s event_id=%s", current, m.EventID)
			case d, ok := <-results:
				if !ok {
					stop()
					return
				}
				var r msg.PollCompleted
				if e = json.Unmarshal(d.Body, &r); e != nil {
					d.Nack(false, false)
					continue
				}
				if r.PollJobID == current && r.DeviceID == "dev-thermostat" && r.CapabilityID == "cap-temperature" && r.Outcome == "succeeded" {
					resultID = r.MeasurementEventID
					log.Printf("poll result poll_job_id=%s event_id=%s", current, r.EventID)
				}
				d.Ack(false)
			case <-tick.C:
				if current != "" && ((measurementID != "" && measurementID == resultID) || time.Now().After(expires)) {
					current = ""
				}
				if !enabled || current != "" || time.Now().Before(next) {
					continue
				}
				call, cancel := context.WithTimeout(ctx, 2*time.Second)
				page, err := devices.ListPollConfigurations(call, &pb.ListPollConfigurationsRequest{})
				cancel()
				if err != nil {
					log.Print(err)
					next = time.Now().Add(time.Second)
					continue
				}
				if len(page.Items) != 1 {
					log.Print("expected one poll configuration")
					continue
				}
				p := page.Items[0]
				c := p.Config
				now := time.Now().UTC()
				expires = now.Add(time.Duration(p.PollTtlMs) * time.Millisecond)
				if expires.After(c.ValidUntil.AsTime()) {
					expires = c.ValidUntil.AsTime()
				}
				if !expires.After(now) {
					continue
				}
				current = msg.ID()
				measurementID = ""
				resultID = ""
				request := msg.PollRequested{Envelope: msg.Event("poll.requested", current, c.DeviceId, c.CapabilityId), ExpiresAt: expires, Config: msg.Config{DeviceID: c.DeviceId, CapabilityID: c.CapabilityId, ConfigVersion: c.ConfigVersion, AdapterKey: c.AdapterKey, EndpointKey: c.EndpointKey, Channel: c.Channel, IssuedAt: c.IssuedAt.AsTime(), ValidUntil: c.ValidUntil.AsTime(), ValueType: c.GetValueType(), Unit: c.GetUnit()}}
				call, cancel = context.WithTimeout(ctx, 2*time.Second)
				err = publisher.Publish(call, "wh.jobs.v1", "poll.requested", request)
				cancel()
				if err != nil {
					log.Print(fmt.Errorf("publish poll: %w", err))
					stop()
					return
				}
				log.Printf("poll requested request_id=%s poll_job_id=%s event_id=%s", request.EventID, current, request.EventID)
				next = now.Add(time.Duration(p.PollIntervalMs) * time.Millisecond)
			}
		}
	}()
	<-ctx.Done()
}
