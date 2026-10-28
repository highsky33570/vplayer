package store

import (
	"fmt"
	"time"
)

// SetVideoMembershipRequired toggles the admin membership gate on a video.
// When enabling, categoryID>0 moves the title into the membership box category.
func (m *MySQL) SetVideoMembershipRequired(videoID uint64, required bool, categoryID uint64) error {
	if videoID == 0 {
		return fmt.Errorf("video id required")
	}
	flag := boolToInt(required)
	if required && categoryID > 0 {
		_, err := m.DB.Exec(
			`UPDATE videos SET membership_required=?, category_id=? WHERE id=?`,
			flag, categoryID, videoID,
		)
		return err
	}
	_, err := m.DB.Exec(`UPDATE videos SET membership_required=? WHERE id=?`, flag, videoID)
	return err
}

// SetUserIsMember toggles site membership for a user (admin-controlled).
// Enabling clears expiry (lifetime grant). Disabling clears both flag and expiry.
func (m *MySQL) SetUserIsMember(userID uint64, isMember bool) error {
	return m.SetUserMembership(userID, isMember, nil)
}

// SetUserMembership sets site membership with an optional expiry (timed VIP).
// isMember with expiresAt stores that expiry; isMember without it is a lifetime
// grant (NULL expiry); !isMember clears both flag and expiry.
func (m *MySQL) SetUserMembership(userID uint64, isMember bool, expiresAt *time.Time) error {
	if userID == 0 {
		return fmt.Errorf("user id required")
	}
	if isMember && expiresAt != nil {
		_, err := m.DB.Exec(`UPDATE users SET is_member=1, membership_expires_at=? WHERE id=?`, *expiresAt, userID)
		return err
	}
	_, err := m.DB.Exec(`UPDATE users SET is_member=?, membership_expires_at=NULL WHERE id=?`, boolToInt(isMember), userID)
	return err
}

// UserSeesMembership reports whether the user may view membership_required titles.
// Admins always see membership titles.
func UserSeesMembership(isMember, isAdmin bool) bool {
	return isMember || isAdmin
}
