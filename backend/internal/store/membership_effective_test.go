package store

import (
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

func TestEffectiveMember_expiry(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	future := now.Add(48 * time.Hour)
	past := now.Add(-time.Hour)

	if !EffectiveMember(&model.User{IsAdmin: true}, now) {
		t.Fatal("admin")
	}
	if !EffectiveMember(&model.User{IsMember: true, MembershipExpiresAt: &future}, now) {
		t.Fatal("paid active")
	}
	if EffectiveMember(&model.User{IsMember: true, MembershipExpiresAt: &past}, now) {
		t.Fatal("expired must not pass")
	}
	if !EffectiveMember(&model.User{IsMember: true}, now) {
		t.Fatal("lifetime admin grant without expiry")
	}
	if MembershipDaysRemaining(&future, now) < 2 {
		t.Fatalf("days=%d", MembershipDaysRemaining(&future, now))
	}
}
