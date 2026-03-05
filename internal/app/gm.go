package app

import (
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
)

// ContactNumberMysteriousInt is the default value for the MysteriousInt field
// in ContactNumber structs used for conversation lookups and message sending.
const ContactNumberMysteriousInt = 7

// NewContactNumbers builds a ContactNumber slice from phone number strings,
// suitable for GetOrCreateConversation requests.
func NewContactNumbers(phones []string) []*gmproto.ContactNumber {
	numbers := make([]*gmproto.ContactNumber, len(phones))
	for i, phone := range phones {
		numbers[i] = &gmproto.ContactNumber{
			MysteriousInt: ContactNumberMysteriousInt,
			Number:        phone,
			Number2:       phone,
		}
	}
	return numbers
}
