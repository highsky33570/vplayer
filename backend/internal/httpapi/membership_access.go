package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

// viewerSeesMembership reports whether the optional Bearer user may see
// membership_required titles. Missing/invalid tokens count as non-members.
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
	return store.UserSeesMembership(user.IsMember, user.IsAdmin)
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
