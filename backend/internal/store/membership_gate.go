package store

import "fmt"

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
func (m *MySQL) SetUserIsMember(userID uint64, isMember bool) error {
	if userID == 0 {
		return fmt.Errorf("user id required")
	}
	_, err := m.DB.Exec(`UPDATE users SET is_member=? WHERE id=?`, boolToInt(isMember), userID)
	return err
}

// UserSeesMembership reports whether the user may view membership_required titles.
// Admins always see membership titles.
func UserSeesMembership(isMember, isAdmin bool) bool {
	return isMember || isAdmin
}
