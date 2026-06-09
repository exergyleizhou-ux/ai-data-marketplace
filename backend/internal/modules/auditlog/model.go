package auditlog

// LogEntry is a read-only view of one audit_logs row.
type LogEntry struct {
	ID           int64          `json:"id"`
	ActorID      string         `json:"actor_id,omitempty"`
	ActorRole    string         `json:"actor_role,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	IP           string         `json:"ip,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	Detail       map[string]any `json:"detail,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

// ListFilter carries optional filter parameters.
type ListFilter struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	From         string
	To           string
	Limit        int
	Offset       int
}
