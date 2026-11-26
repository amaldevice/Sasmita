// Package secrets defines the swappable credential port. Raw keys never enter
// the database, events, logs or the frontend (TAD §15.6).
package secrets

import (
	"context"
	"errors"
)

var (
	ErrNotFound    = errors.New("secret not found")
	ErrUnsupported = errors.New("secret store is not supported on this platform")
)

// Store holds credentials by opaque reference. The UI can ask to store or
// delete a credential but can never read one back through IPC.
type Store interface {
	Put(ctx context.Context, ref, value string) error
	Get(ctx context.Context, ref string) (string, error)
	Delete(ctx context.Context, ref string) error
}
