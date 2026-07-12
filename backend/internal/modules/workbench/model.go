package workbench

import (
	"encoding/json"
	"time"
)

type Workspace struct{ ID, AccountID, Slug, DisplayName, Status string }
type TokenResponse struct {
	Token       string `json:"token"`
	ExpiresIn   int64  `json:"expires_in"`
	WorkspaceID string `json:"workspace_id"`
}

// Owner is the mandatory tenant boundary for every managed-runtime operation.
type Owner struct{ AccountID, WorkspaceID string }

type Run struct {
	ID           string          `json:"id"`
	AccountID    string          `json:"account_id"`
	WorkspaceID  string          `json:"workspace_id"`
	Profile      string          `json:"profile"`
	Status       string          `json:"status"`
	Title        string          `json:"title"`
	Version      int64           `json:"version"`
	Request      json.RawMessage `json:"request"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    *string         `json:"error_code,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
}

type Event struct {
	ID          string          `json:"id"`
	RunID       string          `json:"run_id"`
	AccountID   string          `json:"account_id"`
	WorkspaceID string          `json:"workspace_id"`
	Type        string          `json:"type"`
	Seq         int64           `json:"seq"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Approval struct {
	ApprovalID      string          `json:"approval_id"`
	RunID           string          `json:"run_id"`
	ToolCallID      string          `json:"tool_call_id"`
	AccountID       string          `json:"account_id"`
	WorkspaceID     string          `json:"workspace_id"`
	Owner           string          `json:"owner"`
	RiskLevel       string          `json:"risk_level"`
	Reason          string          `json:"reason"`
	ArgsHash        string          `json:"args_hash"`
	Effects         json.RawMessage `json:"effects"`
	FileScope       json.RawMessage `json:"file_scope"`
	NetworkTargets  json.RawMessage `json:"network_targets"`
	ExpectedOutputs json.RawMessage `json:"expected_outputs"`
	EditableArgs    json.RawMessage `json:"editable_args"`
	Command         *string         `json:"command,omitempty"`
	RemoteTarget    *string         `json:"remote_target,omitempty"`
	Decision        *string         `json:"decision,omitempty"`
	DecidedBy       *string         `json:"decided_by,omitempty"`
	EstimatedCost   int64           `json:"estimated_cost"`
	Version         int64           `json:"version"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	DecidedAt       *time.Time      `json:"decided_at,omitempty"`
}

type Artifact struct {
	ID          string          `json:"id"`
	RunID       string          `json:"run_id"`
	AccountID   string          `json:"account_id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	MediaType   string          `json:"media_type"`
	ObjectKey   string          `json:"-"`
	SHA256      string          `json:"sha256"`
	SizeBytes   int64           `json:"size_bytes"`
	Provenance  json.RawMessage `json:"provenance"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Usage struct {
	RunID, EventID, AccountID, WorkspaceID, Provider, Model string
	InputTokens, OutputTokens, CacheReadTokens              int64
	CacheWriteTokens, CostMicrounits                        int64
	OccurredAt                                              time.Time
}

type ApprovalDecision struct {
	Decision string `json:"decision" binding:"required,oneof=approved rejected"`
	Version  int64  `json:"version" binding:"required,min=1"`
}
