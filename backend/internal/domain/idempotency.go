package domain

import "errors"

// ErrIdempotencyKeyReused: the key was already used by this user for a
// different body (AC-20). The edge maps it to 409.
var ErrIdempotencyKeyReused = errors.New("idempotency key reused with a different body")
