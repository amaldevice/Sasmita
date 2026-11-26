package artifacts

import (
	"errors"
	"strings"
)

// ErrMainFrameNavigation is returned by the host's will-navigate hook when the
// trusted main window tries to load an artifact:// URL. MyGo treats custom
// schemes as trusted in the main frame (research/mygo-isolation.md), so this
// rejection is mandatory rather than a defence-in-depth nicety.
var ErrMainFrameNavigation = errors.New("artifact:// is not allowed in the main frame")

func RejectMainFrameNavigation(rawURL string) error {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if strings.HasPrefix(lower, Scheme+"://") {
		return ErrMainFrameNavigation
	}
	return nil
}
