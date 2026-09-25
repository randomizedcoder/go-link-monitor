package linkmonitor

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

// BaselineStore persists the target port count as a FILENAME in Dir
// (e.g. a file literally named "2"). Load is a single ReadDir; updates
// are a single atomic rename(2).
type BaselineStore struct {
	Dir string
}

// ParseBaselineNames is the pure core of Load: given the entry names of
// the baseline directory, return the baseline value. Multiple numeric
// names can only exist as debris from a crash during the initial create;
// take the max (the strictest target). Non-numeric names are ignored.
func ParseBaselineNames(names []string) (int, bool) {
	best, found := -1, false
	for _, name := range names {
		n, err := strconv.Atoi(name)
		if err != nil || n < 0 {
			continue
		}
		// Reject "+2", "02", " 2" style spellings Atoi accepts but we
		// never write, so debris can't alias a legitimate value.
		if strconv.Itoa(n) != name {
			continue
		}
		found = true
		if n > best {
			best = n
		}
	}
	return best, found
}

// Load returns (value, found, error). A missing directory is "not found",
// not an error.
func (s BaselineStore) Load() (int, bool, error) {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	names := make([]string, len(ents))
	for i, e := range ents {
		names[i] = e.Name()
	}
	n, ok := ParseBaselineNames(names)
	return n, ok, nil
}

// Save records n as the baseline. The first write is create + directory
// fsync (the datum is the dirent itself, so the DIRECTORY must be synced,
// not the file). Subsequent updates atomically rename the old name to the
// new one, so there is no window with zero or two files.
func (s BaselineStore) Save(n int, prev int, hadPrev bool) error {
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return err
	}
	newPath := filepath.Join(s.Dir, strconv.Itoa(n))
	if hadPrev {
		if prev == n {
			return nil
		}
		if err := os.Rename(filepath.Join(s.Dir, strconv.Itoa(prev)), newPath); err != nil {
			return err
		}
	} else {
		f, err := os.OpenFile(newPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if f != nil {
			_ = f.Close()
		}
	}
	return s.syncDir()
}

func (s BaselineStore) syncDir() error {
	d, err := os.Open(s.Dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
