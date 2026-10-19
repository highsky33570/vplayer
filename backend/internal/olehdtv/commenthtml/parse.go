package commenthtml

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	// SourceSystem is the OLEHDTV provider key stored with imported comments.
	SourceSystem = "olehdtv"

	maxAuthorRunes  = 64
	maxAvatarRunes  = 512
	maxBodyRunes    = 4000
	maxCommentIDRun = 32
	maxTimeRunes    = 64
)

// SourceComment is one parent comment or one nested reply from a public comment page.
type SourceComment struct {
	SourceCommentID       string
	SourceVideoID         string
	ParentSourceCommentID string
	AuthorName            string
	AvatarPath            string
	Body                  string
	SourceCreatedRaw      string
	DiggUp                int
	DiggDown              int
}

// Page is the parsed public comment list for one video page.
type Page struct {
	Comments    []SourceComment
	Total       int
	CurrentPage int
	LastPage    int
}

var (
	// ErrMalformedPayload means a JSON-looking body could not be decoded.
	ErrMalformedPayload = errors.New("malformed comment payload")
	// ErrUnexpectedPayload means the body was JSON but not the HTML string this endpoint returns.
	ErrUnexpectedPayload = errors.New("unexpected comment payload")

	reShowQuoted = regexp.MustCompile(`MAC\.Comment\.Show\(\s*'(\d+)'\s*\)`)
	reShowBare   = regexp.MustCompile(`MAC\.Comment\.Show\(\s*(\d+)\s*\)`)
	reFrac       = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
	reHTMLNum    = regexp.MustCompile(`\.html\(\s*['"](\d+)['"]\s*\)`)
	reTotalTips  = regexp.MustCompile(`共\s*(\d+)\s*条`)
	reBreak      = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// Parse accepts either the raw HTML GET body or the XHR body whose JSON value is a string of that HTML.
func Parse(payload, sourceVideoID string) (Page, error) {
	sourceVideoID = strings.TrimSpace(sourceVideoID)
	if sourceVideoID == "" {
		return Page{}, errors.New("missing source video id")
	}
	raw, err := DecodePayload(payload)
	if err != nil {
		return Page{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return Page{}, nil
	}
	nodes, err := html.ParseFragment(strings.NewReader(raw), &html.Node{
		Type: html.ElementNode, DataAtom: atom.Div, Data: "div",
	})
	if err != nil {
		return Page{}, err
	}
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	page := Page{Comments: parseThreads(root, sourceVideoID)}
	total, totalOK, current, last, lastOK := readPager(root)
	parents := 0
	for _, c := range page.Comments {
		if c.ParentSourceCommentID == "" {
			parents++
		}
	}
	if totalOK {
		page.Total = total
	} else if len(page.Comments) > 0 {
		page.Total = len(page.Comments)
	}
	switch {
	case lastOK:
		page.LastPage = last
	case totalOK && parents > 0:
		page.LastPage = (total + parents - 1) / parents
	case len(page.Comments) > 0:
		page.LastPage = 1
	}
	if current > 0 {
		page.CurrentPage = current
	} else if len(page.Comments) > 0 || page.Total > 0 {
		page.CurrentPage = 1
	}
	return page, nil
}

// DecodePayload returns HTML from either a raw document or a JSON string of HTML.
func DecodePayload(payload string) (string, error) {
	s := strings.TrimSpace(payload)
	if s == "" {
		return "", nil
	}
	switch s[0] {
	case '"':
		var inner string
		if err := json.Unmarshal([]byte(s), &inner); err != nil {
			return "", ErrMalformedPayload
		}
		return inner, nil
	case '{', '[':
		return "", ErrUnexpectedPayload
	default:
		return s, nil
	}
}

func parseThreads(root *html.Node, sourceVideoID string) []SourceComment {
	var out []SourceComment
	seen := map[string]bool{}
	var each []*html.Node
	walkElements(root, func(n *html.Node) {
		if n.Data == "li" && hasClass(n, "comm_each") && !insideClass(n.Parent, "comm_each") {
			each = append(each, n)
		}
	})
	for _, li := range each {
		parentID := parentCommentID(li)
		parent := SourceComment{
			SourceCommentID:  parentID,
			SourceVideoID:    sourceVideoID,
			AuthorName:       plainAuthor(directChildText(li, "strong", "text_line")),
			AvatarPath:       cleanAvatar(directAvatar(li)),
			Body:             plainBody(directBody(li)),
			SourceCreatedRaw: plainTime(directChildText(li, "span", "part_tips")),
		}
		parent.DiggUp, parent.DiggDown = directDigg(li)
		if keep(parent, seen) {
			out = append(out, parent)
		}
		var replies []*html.Node
		walkElements(li, func(n *html.Node) {
			if hasClass(n, "comm_reply_child") {
				replies = append(replies, n)
			}
		})
		for _, child := range replies {
			author, when := splitReplyHeader(elementText(firstDesc(child, func(n *html.Node) bool {
				return n.Data == "span" && hasClass(n, "text_line")
			})))
			reply := SourceComment{
				SourceCommentID:       replyID(child),
				SourceVideoID:         sourceVideoID,
				ParentSourceCommentID: parentID,
				AuthorName:            plainAuthor(author),
				AvatarPath: cleanAvatar(attr(firstDesc(child, func(n *html.Node) bool {
					return n.Data == "img" && hasClass(n, "comm_avat")
				}), "src")),
				Body: plainBody(elementText(firstDesc(child, func(n *html.Node) bool {
					return n.Data == "div" && hasClass(n, "comm_content")
				}))),
				SourceCreatedRaw: plainTime(when),
			}
			reply.DiggUp, reply.DiggDown = diggInside(child)
			if parentID == "" || !keep(reply, seen) {
				continue
			}
			out = append(out, reply)
		}
	}
	return out
}

func keep(c SourceComment, seen map[string]bool) bool {
	if c.SourceCommentID == "" || c.Body == "" || seen[c.SourceCommentID] {
		return false
	}
	seen[c.SourceCommentID] = true
	return true
}

func parentCommentID(li *html.Node) string {
	if n := firstDesc(li, func(n *html.Node) bool {
		return n.Data == "a" && hasClass(n, "comment_reply") && !insideClass(n, "comm_reply_child")
	}); n != nil {
		if id := cleanID(attr(n, "data-id")); id != "" {
			return id
		}
	}
	if n := firstDesc(li, func(n *html.Node) bool {
		return n.Data == "a" && hasClass(n, "digg_link") && !insideClass(n, "comm_reply_child")
	}); n != nil {
		return cleanID(attr(n, "data-id"))
	}
	return ""
}

func replyID(child *html.Node) string {
	n := firstDesc(child, func(n *html.Node) bool {
		return n.Data == "a" && hasClass(n, "digg_link")
	})
	return cleanID(attr(n, "data-id"))
}

func directChildText(li *html.Node, tag, class string) string {
	return elementText(firstDesc(li, func(n *html.Node) bool {
		return n.Data == tag && hasClass(n, class) && !insideClass(n, "comm_reply_child")
	}))
}

func directAvatar(li *html.Node) string {
	n := firstDesc(li, func(n *html.Node) bool {
		return n.Data == "img" && hasClass(n, "comm_avat") && !insideClass(n, "comm_reply_child")
	})
	return attr(n, "src")
}

func directBody(li *html.Node) string {
	n := firstDesc(li, func(n *html.Node) bool {
		return n.Data == "div" && hasClass(n, "comm_content") && !insideClass(n, "comm_reply_child")
	})
	return elementText(n)
}

func directDigg(li *html.Node) (int, int) {
	return diggFiltered(li, true)
}

func diggInside(n *html.Node) (int, int) {
	return diggFiltered(n, false)
}

func diggFiltered(root *html.Node, skipReplies bool) (int, int) {
	up, down := 0, 0
	walkElements(root, func(n *html.Node) {
		if n.Data != "a" || !hasClass(n, "digg_link") {
			return
		}
		if skipReplies && insideClass(n, "comm_reply_child") {
			return
		}
		num := diggNumber(n)
		switch attr(n, "data-type") {
		case "up":
			up = num
		case "down":
			down = num
		}
	})
	return up, down
}

func diggNumber(a *html.Node) int {
	em := firstDesc(a, func(n *html.Node) bool {
		return n.Data == "em" && hasClass(n, "digg_num")
	})
	n, err := strconv.Atoi(strings.TrimSpace(elementText(em)))
	if err != nil || n < 0 {
		return 0
	}
	if n > 1_000_000_000 {
		return 1_000_000_000
	}
	return n
}

func readPager(root *html.Node) (total int, totalOK bool, current, last int, lastOK bool) {
	walkElements(root, func(n *html.Node) {
		if n.Data == "script" && strings.Contains(nodeText(n), "mac_total") {
			if m := reHTMLNum.FindStringSubmatch(nodeText(n)); m != nil {
				if v, err := strconv.Atoi(m[1]); err == nil {
					total, totalOK = v, true
				}
			}
		}
		if hasClass(n, "page_tips") && !totalOK {
			if m := reTotalTips.FindStringSubmatch(elementText(n)); m != nil {
				if v, err := strconv.Atoi(m[1]); err == nil {
					total, totalOK = v, true
				}
			}
		}
		if hasClass(n, "num") {
			if m := reFrac.FindStringSubmatch(elementText(n)); m != nil {
				if cur, err := strconv.Atoi(m[1]); err == nil {
					current = cur
				}
				if v, err := strconv.Atoi(m[2]); err == nil && !lastOK {
					last = v
				}
			}
		}
		if n.Data == "a" && (attr(n, "title") == "尾页" || elementText(n) == "尾页") {
			if v, ok := showPage(attr(n, "onclick")); ok {
				last, lastOK = v, true
			}
		}
	})
	if !lastOK && last > 0 {
		lastOK = true
	}
	return total, totalOK, current, last, lastOK
}

func showPage(onclick string) (int, bool) {
	if m := reShowQuoted.FindStringSubmatch(onclick); m != nil {
		v, err := strconv.Atoi(m[1])
		return v, err == nil
	}
	if m := reShowBare.FindStringSubmatch(onclick); m != nil {
		v, err := strconv.Atoi(m[1])
		return v, err == nil
	}
	return 0, false
}

func splitReplyHeader(s string) (string, string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "／"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len("／"):])
	}
	return s, ""
}

func plainAuthor(s string) string {
	return trimRunes(plainCommentText(s), maxAuthorRunes)
}

func plainBody(s string) string {
	return trimRunes(plainCommentText(s), maxBodyRunes)
}

func plainTime(s string) string {
	return trimRunes(strings.TrimSpace(s), maxTimeRunes)
}

func plainCommentText(s string) string {
	s = reBreak.ReplaceAllString(s, " ")
	return maccms.VisibleText(s)
}

func cleanID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxCommentIDRun {
		return ""
	}
	return s
}

func cleanAvatar(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if s == "" || strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") {
		return ""
	}
	return trimRunes(s, maxAvatarRunes)
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}

func elementText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		return ""
	}
	return maccms.VisibleText(b.String())
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

func firstDesc(n *html.Node, pred func(*html.Node) bool) *html.Node {
	if n == nil {
		return nil
	}
	var found *html.Node
	walkElements(n, func(cur *html.Node) {
		if found == nil && cur != n && pred(cur) {
			found = cur
		}
	})
	return found
}

func walkElements(n *html.Node, fn func(*html.Node)) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		fn(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkElements(c, fn)
	}
}

func hasClass(n *html.Node, class string) bool {
	if n == nil {
		return false
	}
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, tok := range strings.Fields(a.Val) {
			if tok == class {
				return true
			}
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func insideClass(n *html.Node, class string) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && hasClass(p, class) {
			return true
		}
	}
	return false
}
