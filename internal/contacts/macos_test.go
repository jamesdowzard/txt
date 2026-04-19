package contacts

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"":                      "",
		"+61437590462":          "437590462",
		"+61 437 590 462":       "437590462",
		"0437590462":            "437590462",
		"0437 590 462":          "437590462",
		"+1 (555) 123-4567":     "551234567",
		"123":                   "", // too short
		"+(61) 4-3-7-5-9-0-462": "437590462",
	}
	for in, want := range cases {
		if got := NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickDisplayName(t *testing.T) {
	cases := []struct {
		first, last, nick, org, want string
	}{
		{"Tommi", "Yick", "", "", "Tommi Yick"},
		{"", "", "TY", "John Holland Group", "TY"},
		{"", "", "", "Optus", "Optus"},
		{"Jarrod", "", "", "Org", "Jarrod"},
		{"  ", "  ", "", "", ""},
	}
	for _, c := range cases {
		got := pickDisplayName(c.first, c.last, c.nick, c.org)
		if got != c.want {
			t.Errorf("pickDisplayName(%q,%q,%q,%q) = %q, want %q",
				c.first, c.last, c.nick, c.org, got, c.want)
		}
	}
}

func TestLoadIndexFrom_MultipleSources(t *testing.T) {
	root := t.TempDir()
	src1 := filepath.Join(root, "src-icloud")
	src2 := filepath.Join(root, "src-onmymac")
	if err := writeFakeAddressBook(t, filepath.Join(src1, "AddressBook-v22.abcddb"), []fakeContact{
		{first: "Tommi", last: "Yick", number: "+61 437 590 462"},
		{first: "Jarrod", last: "McCorkell", org: "John Holland Group", number: "+61410580863"},
		{nick: "Optus", number: "Optus"}, // skipped — no usable digits
	}); err != nil {
		t.Fatalf("write src1: %v", err)
	}
	if err := writeFakeAddressBook(t, filepath.Join(src2, "AddressBook-v22.abcddb"), []fakeContact{
		// Conflicting record for Tommi — should NOT overwrite the iCloud entry
		// because it was indexed first.
		{first: "T", last: "Y", number: "+61437590462"},
		{first: "Sigma", last: "Pervin", number: "0402716236"},
	}); err != nil {
		t.Fatalf("write src2: %v", err)
	}

	idx, err := loadIndexFrom(root)
	if err != nil {
		t.Fatalf("loadIndexFrom: %v", err)
	}

	checks := map[string]string{
		"+61437590462":  "Tommi Yick",
		"0437 590 462":  "Tommi Yick",
		"+61410580863":  "Jarrod McCorkell",
		"0402 716 236":  "Sigma Pervin",
		"+15551234567":  "", // not in index
	}
	for in, want := range checks {
		if got := idx.Lookup(in); got != want {
			t.Errorf("Lookup(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadIndexFrom_EmptyDir(t *testing.T) {
	idx, err := loadIndexFrom(t.TempDir())
	if err != nil {
		t.Fatalf("loadIndexFrom: %v", err)
	}
	if len(idx) != 0 {
		t.Errorf("want empty index, got %d entries", len(idx))
	}
}

type fakeContact struct {
	first, last, nick, org, number string
}

func writeFakeAddressBook(t *testing.T, path string, contacts []fakeContact) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE ZABCDRECORD (Z_PK INTEGER PRIMARY KEY, ZFIRSTNAME TEXT, ZLASTNAME TEXT, ZNICKNAME TEXT, ZORGANIZATION TEXT)`,
		`CREATE TABLE ZABCDPHONENUMBER (Z_PK INTEGER PRIMARY KEY, ZOWNER INTEGER, ZFULLNUMBER TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	for i, c := range contacts {
		if _, err := db.Exec(`INSERT INTO ZABCDRECORD (Z_PK, ZFIRSTNAME, ZLASTNAME, ZNICKNAME, ZORGANIZATION) VALUES (?,?,?,?,?)`,
			i+1, c.first, c.last, c.nick, c.org); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT INTO ZABCDPHONENUMBER (ZOWNER, ZFULLNUMBER) VALUES (?,?)`,
			i+1, c.number); err != nil {
			return err
		}
	}
	return nil
}

