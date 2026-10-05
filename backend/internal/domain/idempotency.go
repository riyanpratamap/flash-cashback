package domain

import "errors"

// ErrIdempotencyKeyReused: the key was already used by this user for a
// different body (AC-20). The edge maps it to 409.
var ErrIdempotencyKeyReused = errors.New("idempotency key reused with a different body")

// ErrInsufficientBalance: the redemption amount is above the balance (AC-30,
// AC-31). The edge maps it to 422.
var ErrInsufficientBalance = errors.New("redemption amount is above the balance")

// ErrRedemptionPaused: redemptions are switched off (AC-32). The edge maps it
// to 409.
var ErrRedemptionPaused = errors.New("redemptions are paused")
