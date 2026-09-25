package linkmonitor

import (
	"strconv"
	"strings"
	"testing"
)

// FuzzParseBaselineNames asserts ParseBaselineNames never panics and always
// returns a canonical, non-negative value that is the max of the acceptable
// names present.
func FuzzParseBaselineNames(f *testing.F) {
	seeds := []string{"", "2", "0", "2\n3", "-1", "02\n+2\n 2", ".keep\nREADME\n7", "9999999999999999999"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, blob string) {
		names := strings.Split(blob, "\n")
		got, found := ParseBaselineNames(names)

		// Independently compute the expected answer: max over names that are
		// the canonical decimal spelling of a non-negative int.
		wantVal, wantFound := -1, false
		for _, name := range names {
			n, err := strconv.Atoi(name)
			if err != nil || n < 0 || strconv.Itoa(n) != name {
				continue
			}
			wantFound = true
			if n > wantVal {
				wantVal = n
			}
		}

		if found != wantFound {
			t.Fatalf("found = %v, want %v (names=%q)", found, wantFound, names)
		}
		if found {
			if got != wantVal {
				t.Fatalf("value = %d, want %d (names=%q)", got, wantVal, names)
			}
			if got < 0 {
				t.Fatalf("returned negative value %d", got)
			}
			if strconv.Itoa(got) == "" {
				t.Fatalf("value %d has no canonical spelling", got)
			}
		}
	})
}

// FuzzPortStateApply drives arbitrary event sequences through PortState and
// asserts the cached up-count invariant holds after every operation, and
// that the count stays within [0, len(map)].
func FuzzPortStateApply(f *testing.F) {
	f.Add([]byte{0x02, 0x81, 0x03, 0xff})
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		s := NewPortState()
		for _, b := range data {
			// Low 4 bits -> index (small, so collisions/flips happen);
			// bit 4 -> deleted; bit 5 -> up.
			ev := LinkEvent{
				Index:   int32(b & 0x0f),
				Deleted: b&0x10 != 0,
				Up:      b&0x20 != 0,
			}
			s.Apply(ev)

			if got, want := s.UpCount(), naiveUpCount(s); got != want {
				t.Fatalf("cached count %d != naive %d after %+v", got, want, ev)
			}
			if s.UpCount() < 0 {
				t.Fatalf("negative count %d", s.UpCount())
			}
			if s.UpCount() > len(s.up) {
				t.Fatalf("count %d exceeds map size %d", s.UpCount(), len(s.up))
			}
		}
	})
}
