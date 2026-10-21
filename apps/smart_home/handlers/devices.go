package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "warmhouse/contracts/gen/warmhouse/v1"
	msg "warmhouse/contracts/messaging"
)

func rpcError(c *gin.Context, err error) {
	code := status.Code(err)
	httpCode := http.StatusInternalServerError
	publicCode := "INTERNAL"
	switch code {
	case codes.AlreadyExists:
		httpCode = 409
		publicCode = "CONFLICT"
	case codes.NotFound:
		httpCode = 404
		publicCode = "NOT_FOUND"
	case codes.InvalidArgument:
		httpCode = 400
		publicCode = "INVALID_ARGUMENT"
	case codes.DeadlineExceeded:
		httpCode = 504
		publicCode = "DEADLINE_EXCEEDED"
	case codes.Unavailable:
		httpCode = 503
		publicCode = "UNAVAILABLE"
	}
	c.JSON(httpCode, gin.H{"code": publicCode, "message": status.Convert(err).Message(), "retryable": code == codes.Unavailable || code == codes.DeadlineExceeded, "request_id": msg.ID()})
}
func RegisterDevices(api *gin.RouterGroup, devices pb.DevicesServiceClient, telemetry pb.TelemetryServiceClient) {
	registerCommands(api, devices)
	api.GET("/devices/:device_id", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		d, e := devices.GetDevice(ctx, &pb.GetDeviceRequest{DeviceId: c.Param("device_id")})
		if e != nil {
			rpcError(c, e)
			return
		}
		capabilities := []any{}
		for _, v := range d.Capabilities {
			m := gin.H{"id": v.Id, "code": v.Code, "kind": v.Kind}
			if v.ValueType != nil {
				m["value_type"] = *v.ValueType
			}
			if v.Unit != nil {
				m["unit"] = *v.Unit
			}
			if v.ParametersSchema != nil {
				m["parameters_schema"] = v.ParametersSchema.AsMap()
			}
			capabilities = append(capabilities, m)
		}
		c.JSON(200, gin.H{"id": d.Id, "house_id": d.HouseId, "type_id": d.TypeId, "name": d.Name, "registration_status": d.RegistrationStatus, "capabilities": capabilities})
	})
	api.GET("/devices/:device_id/telemetry", func(c *gin.Context) {
		r := &pb.ListTelemetryRequest{DeviceId: c.Param("device_id"), Page: &pb.PageRequest{Cursor: c.Query("cursor")}}
		if v, ok := c.GetQuery("capability_id"); ok {
			r.CapabilityId = &v
		}
		if v, ok := c.GetQuery("limit"); ok {
			n, e := strconv.ParseUint(v, 10, 32)
			if e != nil || n == 0 || n > 500 {
				rpcError(c, status.Error(codes.InvalidArgument, "limit must be 1..500"))
				return
			}
			r.Page.Limit = uint32(n)
		}
		for _, f := range []string{"from", "to"} {
			if v, ok := c.GetQuery(f); ok {
				t, e := time.Parse(time.RFC3339Nano, v)
				if e != nil {
					rpcError(c, status.Error(codes.InvalidArgument, "invalid "+f))
					return
				}
				if f == "from" {
					r.From = timestamppb.New(t)
				} else {
					r.To = timestamppb.New(t)
				}
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		p, e := telemetry.ListTelemetry(ctx, r)
		if e != nil {
			rpcError(c, e)
			return
		}
		items := []any{}
		for _, m := range p.Items {
			var value any
			switch v := m.Value.GetValue().(type) {
			case *pb.Value_NumberValue:
				value = v.NumberValue
			case *pb.Value_BooleanValue:
				value = v.BooleanValue
			case *pb.Value_TextValue:
				value = v.TextValue
			}
			item := gin.H{"id": m.Id, "event_id": m.EventId, "device_id": m.DeviceId, "capability_id": m.CapabilityId, "value": value, "measured_at": m.MeasuredAt.AsTime().UTC().Format(time.RFC3339Nano), "gateway_received_at": m.GatewayReceivedAt.AsTime().UTC().Format(time.RFC3339Nano)}
			if m.Unit != nil {
				item["unit"] = *m.Unit
			}
			items = append(items, item)
		}
		result := gin.H{"items": items}
		if p.NextCursor != "" {
			result["next_cursor"] = p.NextCursor
		}
		c.JSON(200, result)
	})
}
