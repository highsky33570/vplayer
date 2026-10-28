package httpapi

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

// membershipRequiredUserMessage is shown when a non-member tries to play a gated title.
const membershipRequiredUserMessage = "需要升级大会员后才能观看"

// viewerSeesMembership reports whether the optional Bearer user may see
// membership_required titles. Missing/invalid tokens count as non-members.
// Uses EffectiveMember so paid expiry (membership_expires_at) is honored.
func (a *API) viewerSeesMembership(c *gin.Context) bool {
	token := auth.BearerToken(c.GetHeader("Authorization"))
	if token == "" {
		return false
	}
	claims, err := auth.ParseToken(a.authSecret(), token)
	if err != nil || claims == nil || claims.UserID == 0 {
		return false
	}
	repo := a.users()
	if repo == nil {
		return false
	}
	user, err := repo.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		return false
	}
	return store.EffectiveMember(user, time.Now())
}

func videoVisibleToViewer(v *model.Video, seesMembership bool) bool {
	if v == nil {
		return false
	}
	if v.MembershipRequired && !seesMembership {
		return false
	}
	return true
}
