package domain

import "github.com/google/uuid"

// MoneyCommand is a validated POST /payments or POST /redemptions request.
// RequestID is the request's X-Request-ID, carried for the money log line.
type MoneyCommand struct {
	UserID    UserID
	Key       uuid.UUID
	Amount    int64
	Hash      [32]byte
	RequestID string
}
