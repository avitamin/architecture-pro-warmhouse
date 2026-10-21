package messaging

import "time"

// CommandEnvelope is separate from the polling envelope: command messages have no poll_job_id.
type CommandEnvelope struct {
	EventID       string    `json:"event_id"`
	Type          string    `json:"type"`
	SchemaVersion int       `json:"schema_version"`
	OccurredAt    time.Time `json:"occurred_at"`
	CommandID     string    `json:"command_id"`
	DeviceID      string    `json:"device_id"`
	CapabilityID  string    `json:"capability_id"`
}
type Parameters struct {
	Operation string         `json:"operation"`
	Arguments map[string]any `json:"arguments"`
}
type CommandRequested struct {
	CommandEnvelope
	UserID                string     `json:"user_id"`
	Source                string     `json:"source"`
	Parameters            Parameters `json:"parameters"`
	AcceptanceSeq         string     `json:"acceptance_seq"`
	ManualCancelBeforeSeq string     `json:"manual_cancel_before_seq"`
	ExpiresAt             time.Time  `json:"expires_at"`
	Config                Config     `json:"config"`
}
type CommandStateChanged struct {
	CommandEnvelope
	ResultVersion  string    `json:"result_version"`
	Status         string    `json:"status"`
	StateChangedAt time.Time `json:"state_changed_at"`
	ReasonCode     string    `json:"reason_code"`
	Evidence       string    `json:"evidence"`
}
