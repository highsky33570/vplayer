package maccms

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	// Locates the start of a player_aaaa object assignment (opening brace).
	rePlayerAAAAAssign = regexp.MustCompile(`(?is)(?:var\s+)?player_aaaa\s*=\s*\{`)
)

// PlayerAAAA is the standard MacCMS frontend player config object.
type PlayerAAAA struct {
	Flag     string          `json:"flag"`
	Encrypt  json.RawMessage `json:"encrypt"`
	Trysee   json.RawMessage `json:"trysee"`
	Points   json.RawMessage `json:"points"`
	Link     string          `json:"link"`
	LinkNext string          `json:"link_next"`
	LinkPrev string          `json:"link_pre"`
	VodData  json.RawMessage `json:"vod_data"`
	URL      string          `json:"url"`
	URLNext  string          `json:"url_next"`
	From     string          `json:"from"`
	SID      flexibleInt     `json:"sid"`
	NID      flexibleInt     `json:"nid"`
	ID       flexibleInt     `json:"id"`
}

type flexibleInt int

func (f *flexibleInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if len(s) > 0 && s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		var n int
		if _, err := fmt.Sscanf(str, "%d", &n); err != nil {
			return err
		}
		*f = flexibleInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexibleInt(n)
	return nil
}

// ParsePlayerAAAA extracts player_aaaa from play-page HTML or raw JS assignment.
func ParsePlayerAAAA(html string) (*PlayerAAAA, bool) {
	raw, ok := ExtractPlayerAAAAJSON(html)
	if !ok {
		return nil, false
	}
	var p PlayerAAAA
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, false
	}
	if p.URL == "" && int(p.ID) == 0 {
		return nil, false
	}
	return &p, true
}

// ExtractPlayerAAAAJSON finds the player_aaaa object via brace-balanced scan.
// Accepts terminators: semicolon, </script>, or end of input (no required ';').
func ExtractPlayerAAAAJSON(html string) (string, bool) {
	loc := rePlayerAAAAAssign.FindStringIndex(html)
	if loc == nil {
		return "", false
	}
	start := loc[1] - 1 // index of '{'
	raw, ok := extractBalancedJSObject(html, start)
	if !ok {
		return "", false
	}
	return sanitizeJSObject(raw), true
}

// extractBalancedJSObject returns the {...} slice starting at start, respecting
// string quotes/escapes so nested braces inside strings are ignored.
func extractBalancedJSObject(s string, start int) (string, bool) {
	if start < 0 || start >= len(s) || s[start] != '{' {
		return "", false
	}
	depth := 0
	inStr := false
	escape := false
	var quote byte
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			quote = c
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// sanitizeJSObject converts common MacCMS JS object literals toward JSON.
func sanitizeJSObject(s string) string {
	s = strings.TrimSpace(s)
	// Replace single-quoted strings with double quotes (simple).
	reSQ := regexp.MustCompile(`'([^'\\]*(?:\\.[^'\\]*)*)'`)
	s = reSQ.ReplaceAllString(s, `"$1"`)
	// Quote bare keys: {url: "x"} -> {"url":"x"}
	reKey := regexp.MustCompile(`([,{]\s*)([A-Za-z_][A-Za-z0-9_]*)\s*:`)
	s = reKey.ReplaceAllString(s, `$1"$2":`)
	return s
}

// ApplyPlayerToEpisode fills stream URL / availability from player_aaaa.
func ApplyPlayerToEpisode(ep *EpisodeRef, p *PlayerAAAA) {
	if ep == nil || p == nil {
		return
	}
	if p.From != "" {
		ep.From = p.From
	}
	if int(p.SID) > 0 {
		ep.SID = int(p.SID)
	}
	if int(p.NID) > 0 {
		ep.NID = int(p.NID)
	}
	if int(p.ID) > 0 && ep.VodID == "" {
		ep.VodID = itoa(int(p.ID))
	}
	url := strings.TrimSpace(p.URL)
	if url == "" {
		ep.Available = false
		return
	}
	// encrypt != 0 usually means URL needs client-side decode — mark unavailable without bypass.
	enc := strings.Trim(string(p.Encrypt), "\" ")
	if enc != "" && enc != "0" && enc != "null" {
		ep.Available = false
		ep.StreamURL = ""
		return
	}
	ep.StreamURL = url
	ep.Available = true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
