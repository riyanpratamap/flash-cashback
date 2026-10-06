package domain

import (
	"encoding/base64"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// CursorType is the history item type in a cursor: P for a payment, R for a
// redemption.
type CursorType byte

const (
	CursorPayment    CursorType = 'P'
	CursorRedemption CursorType = 'R'
)

// rank is the tie-break order of the history key: a payment sorts above a
// redemption at the same instant.
func (t CursorType) rank() int {
	if t == CursorPayment {
		return 1
	}
	return 0
}

// Cursor is the order key (created_at, type rank, id) of the last row of a
// history page. T holds microseconds, as timestamptz does.
type Cursor struct {
	T    time.Time
	Type CursorType
	ID   int64
}

// ErrBadCursor is returned by DecodeCursor for any cursor that is not the one
// canonical encoding of a position.
var ErrBadCursor = errors.New("malformed cursor")

const cursorVersion = "v1"

// EncodeCursor is base64url without padding of v1|<t UTC RFC3339Nano>|<P|R>|<id>.
func EncodeCursor(c Cursor) string {
	raw := cursorVersion + "|" + c.T.UTC().Format(time.RFC3339Nano) + "|" + string(c.Type) + "|" +
		strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor accepts only what EncodeCursor produces.
func DecodeCursor(s string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[0] != cursorVersion {
		return Cursor{}, ErrBadCursor
	}
	if len(parts[2]) != 1 || (parts[2][0] != byte(CursorPayment) && parts[2][0] != byte(CursorRedemption)) {
		return Cursor{}, ErrBadCursor
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || id < 1 {
		return Cursor{}, ErrBadCursor
	}
	t, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil || t.Nanosecond()%1000 != 0 {
		return Cursor{}, ErrBadCursor
	}
	c := Cursor{T: t.UTC(), Type: CursorType(parts[2][0]), ID: id}
	if EncodeCursor(c) != s {
		return Cursor{}, ErrBadCursor
	}
	return c, nil
}

// BranchBound is the id bound for the UNION branch of the given type, in
// `(created_at, id) < (c.T, bound)`. A branch of equal rank continues below
// the cursor id; a lower-rank branch has every row at c.T still to come; a
// higher-rank branch has none left (ids are at least 1).
func BranchBound(c Cursor, branch CursorType) int64 {
	switch {
	case branch.rank() == c.Type.rank():
		return c.ID
	case branch.rank() < c.Type.rank():
		return math.MaxInt64
	default:
		return 0
	}
}
