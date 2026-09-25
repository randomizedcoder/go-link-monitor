package linkmonitor

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseBaselineNames(t *testing.T) {
	cases := []struct {
		description string
		names       []string
		want        int
		wantFound   bool
	}{
		{"empty dir yields not found", nil, 0, false},
		{"empty slice yields not found", []string{}, 0, false},
		{"single value", []string{"2"}, 2, true},
		{"zero is a valid boundary value", []string{"0"}, 0, true},
		{"crash debris takes the max", []string{"2", "3"}, 3, true},
		{"crash debris unordered still takes max", []string{"5", "1", "4"}, 5, true},
		{"junk entries ignored", []string{".keep", "README", "2"}, 2, true},
		{"only junk yields not found", []string{"notes.txt"}, 0, false},
		{"negative rejected", []string{"-1"}, 0, false},
		{"negative alongside valid ignores negative", []string{"-1", "2"}, 2, true},
		{"leading zero non-canonical rejected", []string{"02"}, 0, false},
		{"plus sign non-canonical rejected", []string{"+2"}, 0, false},
		{"leading space non-canonical rejected", []string{" 2"}, 0, false},
		{"non-canonical alongside canonical keeps canonical", []string{"02", "3"}, 3, true},
		{"large value boundary", []string{strconv.Itoa(1 << 30)}, 1 << 30, true},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			got, found := ParseBaselineNames(c.names)
			if found != c.wantFound {
				t.Fatalf("found = %v, want %v", found, c.wantFound)
			}
			if found && got != c.want {
				t.Errorf("value = %d, want %d", got, c.want)
			}
		})
	}
}

func TestBaselineStoreRoundTrip(t *testing.T) {
	s := BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")}

	// Missing directory is "not found", not an error.
	if _, found, err := s.Load(); err != nil || found {
		t.Fatalf("Load on missing dir = found=%v err=%v, want false,nil", found, err)
	}

	// First save creates the file.
	if err := s.Save(2, 0, false); err != nil {
		t.Fatal(err)
	}
	if n, found, err := s.Load(); err != nil || !found || n != 2 {
		t.Fatalf("after first save: Load = (%d,%v,%v), want (2,true,nil)", n, found, err)
	}

	// Update renames atomically: exactly one file afterward.
	if err := s.Save(3, 2, true); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := s.Load(); n != 3 {
		t.Fatalf("after rename: baseline = %d, want 3", n)
	}
	ents, err := os.ReadDir(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "3" {
		t.Fatalf("dir contents after rename = %v, want exactly [3]", ents)
	}

	// Same-value save is a no-op, not an error.
	if err := s.Save(3, 3, true); err != nil {
		t.Fatal(err)
	}

	// Idempotent first-save: O_EXCL collision with existing file is fine.
	if err := s.Save(3, 0, false); err != nil {
		t.Fatal(err)
	}
}

// TestBaselineStoreLoadCrashDebris exercises Load (not just the pure parse)
// against a directory that contains multiple numeric files, as would be left
// by a crash mid-create. Load must return the max.
func TestBaselineStoreLoadCrashDebris(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2", "5", "3", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := BaselineStore{Dir: dir}
	n, found, err := s.Load()
	if err != nil || !found || n != 5 {
		t.Fatalf("Load on debris dir = (%d,%v,%v), want (5,true,nil)", n, found, err)
	}
}

// TestBaselineStoreSaveError exercises the Save error branch: an
// unwritable parent directory makes MkdirAll fail. Skipped as root, which
// bypasses permission checks.
func TestBaselineStoreSaveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permissions")
	}
	parent := t.TempDir()
	ro := filepath.Join(parent, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil { // read+execute, no write
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) }) // let t.TempDir cleanup succeed

	s := BaselineStore{Dir: filepath.Join(ro, "target-ports")}
	if err := s.Save(2, 0, false); err == nil {
		t.Fatal("Save into unwritable parent returned nil error, want failure")
	}
}
