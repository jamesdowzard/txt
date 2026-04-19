// Package contacts builds a phone-number → display-name map by reading the
// macOS Contacts framework's per-source SQLite databases. Used by the
// iMessage importer so chats and messages render with real names instead
// of raw phone numbers.
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

// Index maps the normalised tail of a phone number to the best display name
// the local AddressBook had for it.
type Index map[string]string

// LoadIndex walks every AddressBook source under ~/Library/Application
// Support/AddressBook/Sources and returns a merged phone→name index.
// Sources that fail to open are skipped silently — a missing or locked
// source shouldn't crash the importer.
func LoadIndex() (Index, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home: %w", err)
	}
	return loadIndexFrom(filepath.Join(home, macOSContactsRoot))
}

// loadIndexFrom is the test-friendly entry point that takes the Sources/
// directory directly.
func loadIndexFrom(sourcesDir string) (Index, error) {
	matches, err := filepath.Glob(filepath.Join(sourcesDir, "*", "AddressBook-v22.abcddb"))
	if err != nil {
		return nil, fmt.Errorf("glob abcddb: %w", err)
	}
	idx := Index{}
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
	rows, err := db.Query(`
		SELECT COALESCE(r.ZFIRSTNAME,''), COALESCE(r.ZLASTNAME,''),
			COALESCE(r.ZNICKNAME,''), COALESCE(r.ZORGANIZATION,''),
			COALESCE(p.ZFULLNUMBER,'')
		FROM ZABCDPHONENUMBER p
		JOIN ZABCDRECORD r ON p.ZOWNER = r.Z_PK
	`)
	if err != nil {
		return
	}
	defer rows.Close()
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
		// First win (per-source ordering is stable). Don't clobber an
		// already-mapped name with a blank/org-only entry from a later
		// source.
		if _, ok := idx[key]; !ok {
			idx[key] = name
		}
	}
}

// Lookup returns the resolved name for a phone number, or "" if no match.
func (i Index) Lookup(number string) string {
	if i == nil {
		return ""
	}
	key := NormalizePhone(number)
	if key == "" {
		return ""
	}
	return i[key]
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
