package watchlist

import "errors"

// Watch is a user's bookmark on a dataset.
type Watch struct {
	UserID                string `json:"user_id"`
	DatasetID             string `json:"dataset_id"`
	DatasetTitle          string `json:"dataset_title,omitempty"`
	LastNotifiedVersionID string `json:"last_notified_version_id,omitempty"`
	CreatedAt             string `json:"created_at"`
}

var (
	ErrNotFound = errors.New("watch not found")
)
