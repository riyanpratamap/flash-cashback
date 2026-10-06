package domain

import (
	"encoding/base64"
	"math"
	"testing"
	"time"
)

func enc(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func TestCursorRoundTrip(t *testing.T) {
	us := time.Date(2026, 10, 3, 5, 6, 7, 123456000, time.UTC)
	tests := []struct {
		name string
		c    Cursor
	}{
		{"payment at microseconds", Cursor{us, CursorPayment, 42}},
		{"redemption at microseconds", Cursor{us, CursorRedemption, 3}},
		{"max int64 id", Cursor{us, CursorPayment, math.MaxInt64}},
		{"whole second", Cursor{us.Truncate(time.Second), CursorRedemption, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := EncodeCursor(tt.c)
			got, err := DecodeCursor(s)
			if err != nil {
				t.Fatalf("decode %q: %v", s, err)
			}
			if !got.T.Equal(tt.c.T) || got.Type != tt.c.Type || got.ID != tt.c.ID {
				t.Fatalf("got %+v, want %+v", got, tt.c)
			}
			if EncodeCursor(got) != s {
				t.Fatalf("re-encode differs")
			}
		})
	}
}

func TestEncodeCursorUsesUTC(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	c := Cursor{time.Date(2026, 10, 3, 12, 6, 7, 123456000, wib), CursorPayment, 5}
	want := enc("v1|2026-10-03T05:06:07.123456Z|P|5")
	if got := EncodeCursor(c); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDecodeCursorRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"abc", "abc"},
		{"bang", "!!!"},
		{"padded base64", base64.URLEncoding.EncodeToString([]byte("v1|2026-10-03T05:06:07Z|P|10"))},
		{"v9", enc("v9|x")},
		{"canonical four-field v2", enc("v2|2026-10-03T05:06:07Z|P|1")},
		{"three fields", enc("v1|2026-10-03T05:06:07Z|P")},
		{"five fields", enc("v1|2026-10-03T05:06:07Z|P|1|9")},
		{"type X", enc("v1|2026-10-03T05:06:07Z|X|1")},
		{"id zero", enc("v1|2026-10-03T05:06:07Z|P|0")},
		{"id negative", enc("v1|2026-10-03T05:06:07Z|P|-1")},
		{"id overflow", enc("v1|2026-10-03T05:06:07Z|P|9223372036854775808")},
		{"unparsable t", enc("v1|yesterday|P|1")},
		{"sub-microsecond", enc("v1|2026-10-03T05:06:07.1234567Z|P|1")},
		{"offset not Z", enc("v1|2026-10-03T12:06:07+07:00|P|1")},
		{"trailing zeros", enc("v1|2026-10-03T05:06:07.120000Z|P|1")},
		{"id with plus sign", enc("v1|2026-10-03T05:06:07Z|P|+1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if c, err := DecodeCursor(tt.in); err == nil {
				t.Fatalf("accepted %q as %+v", tt.in, c)
			}
		})
	}
}

func TestBranchBound(t *testing.T) {
	at := time.Date(2026, 10, 3, 5, 6, 7, 0, time.UTC)
	tests := []struct {
		name   string
		c      Cursor
		branch CursorType
		want   int64
	}{
		{"payment cursor, payment branch", Cursor{at, CursorPayment, 42}, CursorPayment, 42},
		{"payment cursor, redemption branch", Cursor{at, CursorPayment, 42}, CursorRedemption, math.MaxInt64},
		{"redemption cursor, redemption branch", Cursor{at, CursorRedemption, 3}, CursorRedemption, 3},
		{"redemption cursor, payment branch", Cursor{at, CursorRedemption, 3}, CursorPayment, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BranchBound(tt.c, tt.branch); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}
