package client

import "strings"

// IsTombstoneStatus reports whether the given status string (as produced by
// MessageStatusType.String()) represents a conversation lifecycle event rather
// than a real message — participant joined/left, conversation created, protocol
// switched, etc. Google Messages synthesises one of these (body:
// "Texting with <name> (SMS/MMS)") for every contact you've ever messaged or
// that the phone considers addressable, which would otherwise pollute the
// conversation list with phantom threads.
func IsTombstoneStatus(status string) bool {
	return strings.Contains(status, "TOMBSTONE")
}
