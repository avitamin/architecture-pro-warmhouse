package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
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

func commandJSON(command *pb.Command) gin.H {
	arguments := gin.H{}
	for k, v := range command.Parameters.Arguments {
		switch x := v.GetValue().(type) {
		case *pb.Value_BooleanValue:
			arguments[k] = x.BooleanValue
		case *pb.Value_NumberValue:
			arguments[k] = x.NumberValue
		case *pb.Value_TextValue:
			arguments[k] = x.TextValue
		}
	}
	body := gin.H{"command_id": command.CommandId, "device_id": command.DeviceId, "capability_id": command.CapabilityId, "user_id": command.UserId, "source": command.Source,
		"parameters": gin.H{"operation": command.Parameters.Operation, "arguments": arguments}, "expires_at": command.ExpiresAt.AsTime().UTC().Format(time.RFC3339Nano),
		"created_at": command.CreatedAt.AsTime().UTC().Format(time.RFC3339Nano), "state_changed_at": command.StateChangedAt.AsTime().UTC().Format(time.RFC3339Nano), "status": command.Status}
	if command.AcceptanceSeq != nil {
		body["acceptance_seq"] = strconv.FormatUint(*command.AcceptanceSeq, 10)
	}
	if command.ResultVersion != nil {
		body["result_version"] = strconv.FormatUint(*command.ResultVersion, 10)
	}
	if command.ReasonCode != nil {
		body["reason_code"] = *command.ReasonCode
	}
	return body
}

func registerCommands(api *gin.RouterGroup, devices pb.DevicesServiceClient) {
	api.POST("/devices/:device_id/commands", func(c *gin.Context) {
		var input struct {
			CommandID    string `json:"command_id"`
			CapabilityID string `json:"capability_id"`
			Parameters   *struct {
				Operation string         `json:"operation"`
				Arguments map[string]any `json:"arguments"`
			} `json:"parameters"`
			ExpiresAt string `json:"expires_at"`
		}
		invalid := func() { rpcError(c, status.Error(codes.InvalidArgument, "invalid command input")) }
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			invalid()
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			invalid()
			return
		}
		expires, err := time.Parse(time.RFC3339Nano, input.ExpiresAt)
		if err != nil || input.Parameters == nil || input.Parameters.Arguments == nil {
			invalid()
			return
		}
		parameters := &pb.Parameters{Operation: input.Parameters.Operation, Arguments: map[string]*pb.Value{}}
		for k, v := range input.Parameters.Arguments {
			value := &pb.Value{}
			switch x := v.(type) {
			case bool:
				value.Value = &pb.Value_BooleanValue{BooleanValue: x}
			case float64:
				value.Value = &pb.Value_NumberValue{NumberValue: x}
			case string:
				value.Value = &pb.Value_TextValue{TextValue: x}
			default:
				invalid()
				return
			}
			parameters.Arguments[k] = value
		}
		requestID := msg.ID()
		c.Header("X-Request-ID", requestID)
		log.Printf("manual command request request_id=%s command_id=%s device_id=%s", requestID, input.CommandID, c.Param("device_id"))
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		result, err := devices.SubmitManualCommand(ctx, &pb.SubmitManualCommandRequest{DeviceId: c.Param("device_id"), Command: &pb.CommandInput{CommandId: input.CommandID, CapabilityId: input.CapabilityID, Parameters: parameters, ExpiresAt: timestamppb.New(expires)}})
		if err != nil {
			rpcError(c, err)
			return
		}
		c.Header("Location", "/api/v1/devices/"+c.Param("device_id")+"/commands/"+result.Command.CommandId)
		code := http.StatusAccepted
		if result.Replayed {
			code = http.StatusOK
		}
		c.JSON(code, commandJSON(result.Command))
	})
	api.GET("/devices/:device_id/commands/:command_id", func(c *gin.Context) {
		requestID := msg.ID()
		c.Header("X-Request-ID", requestID)
		log.Printf("command read request_id=%s command_id=%s device_id=%s", requestID, c.Param("command_id"), c.Param("device_id"))
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		command, err := devices.GetCommand(ctx, &pb.GetCommandRequest{DeviceId: c.Param("device_id"), CommandId: c.Param("command_id")})
		if err != nil {
			rpcError(c, err)
			return
		}
		c.JSON(200, commandJSON(command))
	})
}
