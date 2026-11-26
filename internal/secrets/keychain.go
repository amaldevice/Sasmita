package secrets

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Runner is the narrow seam around the `security` CLI. Tests inject a fake so
// the real Keychain is never touched.
type Runner interface {
	Run(ctx context.Context, args []string, stdin string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(errOut.String()), err)
	}
	return out.Bytes(), nil
}

// Keychain stores credentials in the macOS Keychain via the `security` CLI.
// The secret is supplied on stdin, never in argv where it would be visible in
// the process list.
type Keychain struct {
	Service string
	Runner  Runner
}

func (k Keychain) runner() Runner {
	if k.Runner != nil {
		return k.Runner
	}
	return execRunner{}
}

func (k Keychain) service() string {
	if k.Service != "" {
		return k.Service
	}
	return "Sasmita"
}

// NewKeychain returns the platform store. On non-macOS platforms it returns an
// error-returning store rather than persisting credentials insecurely.
func NewKeychain() Store {
	if runtime.GOOS != "darwin" {
		return unsupported{}
	}
	return Keychain{}
}

type unsupported struct{}

func (unsupported) Put(context.Context, string, string) error { return ErrUnsupported }
func (unsupported) Get(context.Context, string) (string, error) {
	return "", ErrUnsupported
}
func (unsupported) Delete(context.Context, string) error { return ErrUnsupported }

func (k Keychain) Put(ctx context.Context, ref, value string) error {
	_, err := k.runner().Run(ctx, []string{
		"add-generic-password", "-a", ref, "-s", k.service(), "-T", "", "-U", "-w",
	}, value)
	return err
}

func (k Keychain) Get(ctx context.Context, ref string) (string, error) {
	out, err := k.runner().Run(ctx, []string{
		"find-generic-password", "-a", ref, "-s", k.service(), "-w",
	}, "")
	if err != nil {
		if strings.Contains(err.Error(), "could not be found") {
			return "", ErrNotFound
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (k Keychain) Delete(ctx context.Context, ref string) error {
	_, err := k.runner().Run(ctx, []string{
		"delete-generic-password", "-a", ref, "-s", k.service(),
	}, "")
	return err
}
