package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/google/uuid"
)

// UserID is a validated X-User-ID value.
type UserID string

// Validation failures. The HTTP edge maps each to a contract code.
var (
	ErrMissingUser           = errors.New("missing user")
	ErrInvalidUser           = errors.New("invalid user")
	ErrMissingIdempotencyKey = errors.New("missing idempotency key")
	ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")
	ErrMalformedRequest      = errors.New("malformed request")
	ErrInvalidAmount         = errors.New("invalid amount")
)

const (
	maxUserIDLen    = 64
	uuidLen         = 36
	minAmount       = 1
	maxAmount       = 10_000_000
	defaultLimit    = 20
	maxLimit        = 50
	amountKey       = "amount"
	requestHashBase = "amount="
)

// ParseUserID validates the X-User-ID header values: one value of 1 to 64
// characters from [a-z0-9_-].
func ParseUserID(vals []string) (UserID, error) {
	if len(vals) == 0 {
		return "", ErrMissingUser
	}
	if len(vals) != 1 || !validUserID(vals[0]) {
		return "", ErrInvalidUser
	}
	return UserID(vals[0]), nil
}

func validUserID(s string) bool {
	if len(s) < 1 || len(s) > maxUserIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// ParseIdempotencyKey accepts only the canonical 8-4-4-4-12 form; uuid.Parse
// alone also accepts braced, urn: and dash-less forms (KP).
func ParseIdempotencyKey(vals []string) (uuid.UUID, error) {
	if len(vals) == 0 {
		return uuid.UUID{}, ErrMissingIdempotencyKey
	}
	if len(vals) != 1 || !canonicalUUID(vals[0]) {
		return uuid.UUID{}, ErrInvalidIdempotencyKey
	}
	id, err := uuid.Parse(vals[0])
	if err != nil {
		return uuid.UUID{}, ErrInvalidIdempotencyKey
	}
	return id, nil
}

func canonicalUUID(s string) bool {
	if len(s) != uuidLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !isHex(c) {
			return false
		}
	}
	return true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// ParseAmountBody reads {"amount": <number>} by walking the raw tokens, so a
// quoted value, a duplicate key, a differently-cased key, or trailing data is
// ErrMalformedRequest, and a number that is not a whole 1..10,000,000 is
// ErrInvalidAmount. It never decodes into any (KP).
func ParseAmountBody(body []byte) (int64, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return 0, ErrMalformedRequest
	}
	if tok, err := dec.Token(); err != nil || tok != amountKey {
		return 0, ErrMalformedRequest
	}
	tok, err := dec.Token()
	if err != nil {
		return 0, ErrMalformedRequest
	}
	num, ok := tok.(json.Number)
	if !ok {
		return 0, ErrMalformedRequest
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return 0, ErrMalformedRequest
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return 0, ErrMalformedRequest
	}

	text := num.String()
	if !allDigits(text) {
		return 0, ErrInvalidAmount
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < minAmount || n > maxAmount {
		return 0, ErrInvalidAmount
	}
	return n, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ParseLimit reads the history page size: absent is 20, else digits 1..50.
func ParseLimit(raw string, present bool) (int, error) {
	if !present {
		return defaultLimit, nil
	}
	if !allDigits(raw) {
		return 0, ErrMalformedRequest
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxLimit {
		return 0, ErrMalformedRequest
	}
	return n, nil
}

// RequestHash is sha256("amount=" + decimal amount), stored with the row an
// idempotent request creates (tech-spec 4.1 step 1).
func RequestHash(amount int64) [32]byte {
	return sha256.Sum256([]byte(requestHashBase + strconv.FormatInt(amount, 10)))
}
