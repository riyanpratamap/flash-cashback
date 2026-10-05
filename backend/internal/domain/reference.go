package domain

import (
	"fmt"
	"time"
)

// RefKind is the prefix of a payment or redemption reference.
type RefKind string

const (
	RefPayment    RefKind = "PAY"
	RefRedemption RefKind = "RDM"
)

// wib is a fixed UTC+7 zone; WIB has no DST (D19).
var wib = time.FixedZone("WIB", 7*3600)

// Reference builds PAY-20261003-000042 from the campaign day (a DB date, so
// its own year, month and day are used with no zone conversion) and the ID.
func Reference(kind RefKind, campaignDay time.Time, id int64) string {
	y, m, d := campaignDay.Date()
	return fmt.Sprintf("%s-%04d%02d%02d-%06d", kind, y, int(m), d, id)
}

// FormatTime renders an instant as RFC 3339 in WIB at second precision.
func FormatTime(t time.Time) string { return t.In(wib).Format(time.RFC3339) }
