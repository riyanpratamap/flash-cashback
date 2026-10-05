package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseUserID(t *testing.T) {
	tests := []struct {
		name string
		vals []string
		want UserID
		err  error
	}{
		{"absent", nil, "", ErrMissingUser},
		{"empty list", []string{}, "", ErrMissingUser},
		{"valid", []string{"user_a"}, "user_a", nil},
		{"dash and digits", []string{"u-1_2"}, "u-1_2", nil},
		{"64 chars", []string{strings.Repeat("a", 64)}, UserID(strings.Repeat("a", 64)), nil},
		{"65 chars", []string{strings.Repeat("a", 65)}, "", ErrInvalidUser},
		{"upper case", []string{"User_A"}, "", ErrInvalidUser},
		{"empty value", []string{""}, "", ErrInvalidUser},
		{"space", []string{"a b"}, "", ErrInvalidUser},
		{"trailing newline", []string{"a\n"}, "", ErrInvalidUser},
		{"two values", []string{"a", "b"}, "", ErrInvalidUser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUserID(tc.vals)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestParseIdempotencyKey(t *testing.T) {
	const canon = "123e4567-e89b-12d3-a456-426614174000"
	tests := []struct {
		name string
		vals []string
		err  error
	}{
		{"absent", nil, ErrMissingIdempotencyKey},
		{"canonical", []string{canon}, nil},
		{"upper-case hex", []string{strings.ToUpper(canon)}, nil},
		{"abc", []string{"abc"}, ErrInvalidIdempotencyKey},
		{"empty", []string{""}, ErrInvalidIdempotencyKey},
		{"braced", []string{"{" + canon + "}"}, ErrInvalidIdempotencyKey},
		{"urn", []string{"urn:uuid:" + canon}, ErrInvalidIdempotencyKey},
		{"no dashes", []string{strings.ReplaceAll(canon, "-", "")}, ErrInvalidIdempotencyKey},
		{"36 chars, dashes misplaced (rejected by the hex check, not the dash check)", []string{"123e4567e-89b-12d3-a456-426614174000"}, ErrInvalidIdempotencyKey},
		{"non-hex", []string{"123e4567-e89b-12d3-a456-42661417400g"}, ErrInvalidIdempotencyKey},
		{"two values", []string{canon, canon}, ErrInvalidIdempotencyKey},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseIdempotencyKey(tc.vals)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if tc.err == nil && got != uuid.MustParse(canon) {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestParseAmountBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int64
		err  error
	}{
		{"valid", `{"amount":100000}`, 100000, nil},
		{"whitespace", " {\n \"amount\" : 20000 }\n", 20000, nil},
		{"min", `{"amount":1}`, 1, nil},
		{"max", `{"amount":10000000}`, 10000000, nil},
		{"zero", `{"amount":0}`, 0, ErrInvalidAmount},
		{"negative", `{"amount":-1}`, 0, ErrInvalidAmount},
		{"above max", `{"amount":10000001}`, 0, ErrInvalidAmount},
		{"fraction", `{"amount":1.5}`, 0, ErrInvalidAmount},
		{"trailing zero fraction", `{"amount":100000.0}`, 0, ErrInvalidAmount},
		{"exponent", `{"amount":1e5}`, 0, ErrInvalidAmount},
		{"int64 overflow", `{"amount":99999999999999999999}`, 0, ErrInvalidAmount},
		{"string", `{"amount":"100000"}`, 0, ErrMalformedRequest},
		{"null", `{"amount":null}`, 0, ErrMalformedRequest},
		{"true", `{"amount":true}`, 0, ErrMalformedRequest},
		{"object value", `{"amount":{}}`, 0, ErrMalformedRequest},
		{"array value", `{"amount":[1]}`, 0, ErrMalformedRequest},
		{"empty object", `{}`, 0, ErrMalformedRequest},
		{"missing amount", `{"other":1}`, 0, ErrMalformedRequest},
		{"unknown field", `{"amount":1,"x":2}`, 0, ErrMalformedRequest},
		{"capitalised key", `{"Amount":1}`, 0, ErrMalformedRequest},
		{"duplicate key", `{"amount":1,"amount":2}`, 0, ErrMalformedRequest},
		{"non-JSON", `amount=1`, 0, ErrMalformedRequest},
		{"empty body", ``, 0, ErrMalformedRequest},
		{"array body", `[1]`, 0, ErrMalformedRequest},
		{"bare number", `5`, 0, ErrMalformedRequest},
		{"trailing data", `{"amount":1}{"amount":2}`, 0, ErrMalformedRequest},
		{"trailing garbage", `{"amount":1}x`, 0, ErrMalformedRequest},
		{"truncated", `{"amount":1`, 0, ErrMalformedRequest},
		{"leading zero", `{"amount":01}`, 0, ErrMalformedRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAmountBody([]byte(tc.body))
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("got (%d, %v), want (%d, %v)", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		present bool
		want    int
		err     error
	}{
		{"absent", "", false, 20, nil},
		{"1", "1", true, 1, nil},
		{"20", "20", true, 20, nil},
		{"50", "50", true, 50, nil},
		{"0", "0", true, 0, ErrMalformedRequest},
		{"51", "51", true, 0, ErrMalformedRequest},
		{"abc", "abc", true, 0, ErrMalformedRequest},
		{"plus", "+5", true, 0, ErrMalformedRequest},
		{"negative", "-1", true, 0, ErrMalformedRequest},
		{"empty present", "", true, 0, ErrMalformedRequest},
		{"huge", "99999999999999999999", true, 0, ErrMalformedRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLimit(tc.raw, tc.present)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("got (%d, %v), want (%d, %v)", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestRequestHash(t *testing.T) {
	// sha256("amount=100000")
	const want = "a5b432c76c2da2d145b031b654459b0e82a04a9eb2f060906e6ade7de652f0fa"
	got := RequestHash(100000)
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("hash = %x", got)
	}
	if RequestHash(100000) == RequestHash(100001) {
		t.Fatal("different amounts share a hash")
	}
}
