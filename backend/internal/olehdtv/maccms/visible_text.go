package maccms

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// nonVisibleHTMLTags are elements whose text must never become metadata.
var nonVisibleHTMLTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"template": true,
}

func fragmentContext() *html.Node {
	return &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Div,
		Data:     "div",
	}
}

// VisibleText returns human-visible text from an HTML fragment.
// It walks DOM text nodes and ignores script/style/noscript/template subtrees.
func VisibleText(fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), fragmentContext())
	if err != nil {
		return collapseWS(decodeBasicEntities(fragment))
	}
	var b strings.Builder
	for _, n := range nodes {
		appendVisibleText(&b, n)
	}
	return collapseWS(b.String())
}

// StripNonVisibleElements removes script/style/noscript/template subtrees from an
// HTML fragment and returns the remaining markup (for regex metadata scopes).
func StripNonVisibleElements(fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), fragmentContext())
	if err != nil {
		return fragment
	}
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	for _, n := range nodes {
		root.AppendChild(cloneWithoutNonVisible(n))
	}
	var b strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return fragment
		}
	}
	return b.String()
}

func appendVisibleText(b *strings.Builder, n *html.Node) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.ElementNode:
		if nonVisibleHTMLTags[strings.ToLower(n.Data)] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			appendVisibleText(b, c)
		}
	case html.TextNode:
		b.WriteString(n.Data)
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			appendVisibleText(b, c)
		}
	}
}

func cloneWithoutNonVisible(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && nonVisibleHTMLTags[strings.ToLower(n.Data)] {
		return &html.Node{Type: html.TextNode, Data: ""}
	}
	out := &html.Node{
		Type:     n.Type,
		DataAtom: n.DataAtom,
		Data:     n.Data,
		Attr:     append([]html.Attribute(nil), n.Attr...),
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out.AppendChild(cloneWithoutNonVisible(c))
	}
	return out
}

func collapseWS(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = reWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func decodeBasicEntities(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	return s
}
