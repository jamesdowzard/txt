package client

import "testing"

func TestIsTombstoneStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"TOMBSTONE_ONE_ON_ONE_SMS_CREATED", true},
		{"TOMBSTONE_ONE_ON_ONE_RCS_CREATED", true},
		{"TOMBSTONE_RCS_GROUP_CREATED", true},
		{"TOMBSTONE_PARTICIPANT_JOINED", true},
		{"MESSAGE_STATUS_TOMBSTONE_PARTICIPANT_REMOVED_FROM_GROUP", true},
		{"OUTGOING_COMPLETE", false},
		{"INCOMING_COMPLETE", false},
		{"MESSAGE_DELETED", false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		if got := IsTombstoneStatus(tc.status); got != tc.want {
			t.Errorf("IsTombstoneStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
