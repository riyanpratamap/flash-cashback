package domain

import (
	"testing"
	"time"
)

func TestReference(t *testing.T) {
	day := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		kind RefKind
		day  time.Time
		id   int64
		want string
	}{
		{"padded to six", RefPayment, day, 42, "PAY-20261003-000042"},
		{"grows past six", RefPayment, day, 1234567, "PAY-20261003-1234567"},
		{"redemption", RefRedemption, day, 7, "RDM-20261003-000007"},
		{"day uses its own date in any zone", RefPayment, time.Date(2026, 10, 3, 23, 0, 0, 0, wib), 1, "PAY-20261003-000001"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Reference(tc.kind, tc.day, tc.id); got != tc.want {
				t.Errorf("Reference = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"last second of day", time.Date(2026, 10, 3, 16, 59, 59, 0, time.UTC), "2026-10-03T23:59:59+07:00"},
		{"day boundary", time.Date(2026, 10, 3, 17, 0, 0, 0, time.UTC), "2026-10-04T00:00:00+07:00"},
		{"midnight UTC", time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), "2026-10-03T07:00:00+07:00"},
		{"sub-second dropped", time.Date(2026, 10, 3, 17, 0, 0, 987654321, time.UTC), "2026-10-04T00:00:00+07:00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatTime(tc.in); got != tc.want {
				t.Errorf("FormatTime = %q, want %q", got, tc.want)
			}
		})
	}
}
