package olehdtv

import "testing"

func TestValidateSourceID(t *testing.T) {
	ok := []string{"1", "84839", "999999"}
	for _, id := range ok {
		if err := ValidateSourceID(id); err != nil {
			t.Fatalf("%q: %v", id, err)
		}
	}
	bad := []string{"", "  ", "0", "-1", "abc", "12.3", "01x"}
	for _, id := range bad {
		if err := ValidateSourceID(id); err == nil {
			t.Fatalf("%q: expected error", id)
		}
	}
}
