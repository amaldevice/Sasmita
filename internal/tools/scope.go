package tools

import "context"

// Scope is the per-run resource boundary. Roots come from the conversation's
// project; attached files are added explicitly. A conversation without a
// project has no roots, so file tools can only read attached paths.
type Scope struct {
	Roots    []string
	Attached []string
}

type scopeKey struct{}

// WithScope attaches the derived scope to the run context. The registry's
// construction-time roots remain a fallback for callers that do not create a
// run (direct tool tests), but every appsvc run supplies its own scope.
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopeFrom reports the scope carried by ctx, if any.
func ScopeFrom(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	return scope, ok
}
