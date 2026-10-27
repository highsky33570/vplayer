package olehdtv

import "testing"

func TestIsVIPSourceCategoryID(t *testing.T) {
	for _, id := range []string{"6", "601", "613", " 602 "} {
		if !IsVIPSourceCategoryID(id) {
			t.Fatalf("%q should be VIP", id)
		}
	}
	for _, id := range []string{"", "1", "5", "101", "506", "7"} {
		if IsVIPSourceCategoryID(id) {
			t.Fatalf("%q must not be VIP", id)
		}
	}
}
