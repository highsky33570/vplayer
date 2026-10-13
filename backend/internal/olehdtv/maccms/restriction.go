package maccms

import "strings"

// PlayPageClass is a safe internal classification of a fetched play-page body.
// Values are machine-oriented; never store raw upstream HTML or membership copy.
type PlayPageClass string

const (
	PlayPageOK               PlayPageClass = "ok"
	PlayPageSourceRestricted PlayPageClass = "source_restricted"
	PlayPageUnknown          PlayPageClass = "unknown"
)

// Confirmed OLEHDTV membership / permission denial markers (HTTP 200 HTML).
// Require the distinctive permission phrase; corroborate with system-prompt or upgrade cue
// so unrelated Chinese copy does not classify as restricted.
const (
	oleHdTvPermissionPhrase = "您没有权限访问此数据"
	oleHdTvUpgradeCue       = "请升级会员"
	oleHdTvSystemPromptCue  = "系统提示"
)

// IsSourceRestrictedPage reports a confirmed OLEHDTV permission/membership denial page.
// It does not treat arbitrary missing player_aaaa HTML as restricted.
func IsSourceRestrictedPage(html string) bool {
	if !strings.Contains(html, oleHdTvPermissionPhrase) {
		return false
	}
	return strings.Contains(html, oleHdTvUpgradeCue) || strings.Contains(html, oleHdTvSystemPromptCue)
}

// ClassifyPlayPage classifies play-page HTML before/without relying on HTTP status.
// Order: source restriction → usable player_aaaa → unknown (retryable).
func ClassifyPlayPage(html string) PlayPageClass {
	if IsSourceRestrictedPage(html) {
		return PlayPageSourceRestricted
	}
	if _, ok := ParsePlayerAAAA(html); ok {
		return PlayPageOK
	}
	return PlayPageUnknown
}
