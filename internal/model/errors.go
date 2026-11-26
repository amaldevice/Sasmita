package model

import "errors"

// ErrUnauthorized means the provider rejected the credential (HTTP 401/403).
// Appservices map it to a plain-language "bad key" result.
var ErrUnauthorized = errors.New("provider rejected the credentials")

// ErrUnreachable means the provider could not be reached at all. Appservices
// map it to a plain-language "unreachable host" result.
var ErrUnreachable = errors.New("provider is unreachable")
