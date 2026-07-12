package workbench

type Workspace struct{ ID, AccountID, Slug, DisplayName, Status string }
type TokenResponse struct {
	Token       string `json:"token"`
	ExpiresIn   int64  `json:"expires_in"`
	WorkspaceID string `json:"workspace_id"`
}
