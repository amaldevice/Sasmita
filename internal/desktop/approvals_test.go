package desktop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
)

// The typed shell surface must expose approval decisions to Svelte. This is
// the desktop-side counterpart of the seam-1 appsvc commands.
func TestBindingsExposeApprovalCommands(t *testing.T) {
	service := newShellBindings(t)
	var surface desktop.Bindings = service
	if pending, err := surface.ListPendingApprovals(context.Background(), "c1"); err != nil || len(pending) != 0 {
		t.Fatalf("pending approvals = %+v (err %v)", pending, err)
	}
	if err := surface.ResolveApproval(context.Background(), "missing", domain.ApprovalAllowOnce, ""); !errors.Is(err, domain.ErrApprovalNotFound) {
		t.Fatalf("ResolveApproval(missing) = %v, want ErrApprovalNotFound", err)
	}
}
