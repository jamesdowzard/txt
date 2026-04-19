// Package contacts builds a handle → display-name index by reading the macOS
// Contacts framework's per-source SQLite databases. Used by the iMessage
// importer so chats and messages render with real names instead of raw
// phone numbers or email addresses.
package contacts

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// macOSContactsRoot is the directory holding one AddressBook-v22.abcddb per
// account (iCloud, On My Mac, Exchange, etc).
const macOSContactsRoot = "Library/Application Support/AddressBook/Sources"

// phoneKeyLen is how many trailing digits we use to match a number across
// formats. 9 digits is enough to disambiguate within a country while
// surviving "+61 437 590 462" / "0437 590 462" / "+61437590462" variations.
const phoneKeyLen = 9

// Index resolves either a phone number or an email address to a display name.
// Phone keys are NormalizePhone(...); email keys are strings.ToLower(...).
type Index struct {
	Phones map[string]string
	Emails map[string]string
}

// NewIndex returns an empty, ready-to-write index.
func NewIndex() Index {
	return Index{Phones: map[string]string{}, Emails: map[string]string{}}
}

// LoadIndex walks every AddressBook source under ~/Library/Application
// Support/AddressBook/Sources and returns a merged handle→name index.
// Sources that fail to open are skipped silently — a missing or locked
// source shouldn't crash the importer.
func LoadIndex() (Index, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return NewIndex(), fmt.Errorf("user home: %w", err)
	}
	return loadIndexFrom(filepath.Join(home, macOSContactsRoot))
}

// loadIndexFrom is the test-friendly entry point that takes the Sources/
// directory directly.
func loadIndexFrom(sourcesDir string) (Index, error) {
	matches, err := filepath.Glob(filepath.Join(sourcesDir, "*", "AddressBook-v22.abcddb"))
	if err != nil {
		return NewIndex(), fmt.Errorf("glob abcddb: %w", err)
	}
	idx := NewIndex()
	for _, dbPath := range matches {
		readSource(dbPath, idx)
	}
	return idx, nil
}

func readSource(dbPath string, idx Index) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return
	}
	defer db.Close()

	// Phones
	rows, err := db.Query(`
		SELECT COALESCE(r.ZFIRSTNAME,''), COALESCE(r.ZLASTNAME,''),
			COALESCE(r.ZNICKNAME,''), COALESCE(r.ZORGANIZATION,''),
			COALESCE(p.ZFULLNUMBER,'')
		FROM ZABCDPHONENUMBER p
		JOIN ZABCDRECORD r ON p.ZOWNER = r.Z_PK
	`)
	if err == nil {
		for rows.Next() {
			var first, last, nick, org, number string
			if err := rows.Scan(&first, &last, &nick, &org, &number); err != nil {
				continue
			}
			key := NormalizePhone(number)
			if key == "" {
				continue
			}
			name := pickDisplayName(first, last, nick, org)
			if name == "" {
				continue
			}
			if _, ok := idx.Phones[key]; !ok {
				idx.Phones[key] = name
			}
		}
		rows.Close()
	}

	// Emails
	emailRows, err := db.Query(`
		SELECT COALESCE(r.ZFIRSTNAME,''), COALESCE(r.ZLASTNAME,''),
			COALESCE(r.ZNICKNAME,''), COALESCE(r.ZORGANIZATION,''),
			COALESCE(e.ZADDRESS,'')
		FROM ZABCDEMAILADDRESS e
		JOIN ZABCDRECORD r ON e.ZOWNER = r.Z_PK
	`)
	if err == nil {
		for emailRows.Next() {
			var first, last, nick, org, address string
			if err := emailRows.Scan(&first, &last, &nick, &org, &address); err != nil {
				continue
			}
			key := normalizeEmail(address)
			if key == "" {
				continue
			}
			name := pickDisplayName(first, last, nick, org)
			if name == "" {
				continue
			}
			if _, ok := idx.Emails[key]; !ok {
				idx.Emails[key] = name
			}
		}
		emailRows.Close()
	}
}

// Lookup returns the resolved name for a chat.db handle, or "" if no match.
// Detects email vs phone by the presence of '@'.
func (i Index) Lookup(handle string) string {
	if handle == "" {
		return ""
	}
	if strings.Contains(handle, "@") {
		if i.Emails == nil {
			return ""
		}
		return i.Emails[normalizeEmail(handle)]
	}
	if i.Phones == nil {
		return ""
	}
	key := NormalizePhone(handle)
	if key == "" {
		return ""
	}
	return i.Phones[key]
}

// NormalizePhone reduces a phone number to its last phoneKeyLen digits,
// stripping every non-digit. Returns "" for inputs with fewer than
// phoneKeyLen digits (too ambiguous to match safely).
func NormalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if len(digits) < phoneKeyLen {
		return ""
	}
	return digits[len(digits)-phoneKeyLen:]
}

func normalizeEmail(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if !strings.Contains(s, "@") {
		return ""
	}
	return s
}

func pickDisplayName(first, last, nick, org string) string {
	if nick = strings.TrimSpace(nick); nick != "" {
		return nick
	}
	full := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	if full != "" {
		return full
	}
	return strings.TrimSpace(org)
}
