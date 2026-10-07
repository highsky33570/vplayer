package maccms

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveIndex maps original page URLs to files inside a SiteOne (or similar) offline export.
type ArchiveIndex struct {
	Root    string
	BaseURL string
	ByURL   map[string]string // absolute or path URL -> relative file path
	Files   int
}

// BuildArchiveIndex walks an offline export directory and records MacCMS-relevant HTML files.
// Supports:
//   - VPlayer fixture layout (type/1/page-1.html, detail/10001.html, ...)
//   - SiteOne --offline-export-preserve-url-structure (index.php/vod/detail/id/10001.html)
//   - Optional url-map.json: { "https://host/path": "relative/file.html" }
func BuildArchiveIndex(root, baseURL string) (*ArchiveIndex, error) {
	idx := &ArchiveIndex{
		Root:    root,
		BaseURL: strings.TrimRight(baseURL, "/"),
		ByURL:   map[string]string{},
	}
	if b, err := os.ReadFile(filepath.Join(root, "url-map.json")); err == nil {
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			for u, rel := range m {
				idx.add(u, rel)
			}
		}
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".html" && ext != ".htm" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		idx.Files++
		idx.indexRelative(rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return idx, nil
}

func (idx *ArchiveIndex) indexRelative(rel string) {
	idx.addPathVariants(rel)

	// Fixture shortcuts
	if rel == "home.html" || rel == "index.html" {
		idx.add(idx.BaseURL+"/", rel)
		idx.add(idx.BaseURL+"/index.html", rel)
		idx.add(idx.BaseURL+"/index.php", rel)
	}
	// type/{id}/page-{n}.html — only page-1 maps to the bare type URL
	if strings.HasPrefix(rel, "type/") {
		parts := strings.Split(rel, "/")
		if len(parts) >= 3 {
			tid := parts[1]
			base := strings.TrimSuffix(parts[len(parts)-1], ".html")
			page := 1
			if strings.HasPrefix(base, "page-") {
				fmt.Sscanf(base, "page-%d", &page)
			}
			paged := fmt.Sprintf("%s/index.php/vod/type/id/%s/page/%d.html", idx.BaseURL, tid, page)
			idx.add(paged, rel)
			if page <= 1 {
				idx.add(fmt.Sprintf("%s/index.php/vod/type/id/%s.html", idx.BaseURL, tid), rel)
			}
		}
	}
	if strings.HasPrefix(rel, "detail/") {
		id := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		idx.add(fmt.Sprintf("%s/index.php/vod/detail/id/%s.html", idx.BaseURL, id), rel)
	}
	if strings.HasPrefix(rel, "play/") {
		// 10001-s1-n1.html
		base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		var vod string
		var sid, nid int
		if _, err := fmt.Sscanf(base, "%s-s%d-n%d", &vod, &sid, &nid); err == nil || strings.Contains(base, "-s") {
			// Sscanf with %s is greedy; parse manually
			if i := strings.Index(base, "-s"); i > 0 {
				vod = base[:i]
				fmt.Sscanf(base[i:], "-s%d-n%d", &sid, &nid)
				idx.add(fmt.Sprintf("%s/index.php/vod/play/id/%s/sid/%d/nid/%d.html", idx.BaseURL, vod, sid, nid), rel)
			}
		}
	}
}

func (idx *ArchiveIndex) addPathVariants(rel string) {
	// SiteOne preserve-url-structure: index.php/vod/detail/id/1.html
	slash := "/" + strings.TrimPrefix(rel, "/")
	idx.add(idx.BaseURL+slash, rel)
	idx.add(slash, rel)
	if strings.HasSuffix(rel, "/index.html") {
		dir := strings.TrimSuffix(rel, "/index.html")
		idx.add(idx.BaseURL+"/"+dir, rel)
		idx.add(idx.BaseURL+"/"+dir+"/", rel)
	}
}

func (idx *ArchiveIndex) add(u, rel string) {
	u = strings.TrimSpace(u)
	if u == "" || rel == "" {
		return
	}
	idx.ByURL[normalizeVisitKey(u)] = rel
	// also without trailing slash variants
	idx.ByURL[normalizeVisitKey(strings.TrimRight(u, "/"))] = rel
}

func (idx *ArchiveIndex) Resolve(pageURL string) (string, bool) {
	key := normalizeVisitKey(pageURL)
	if rel, ok := idx.ByURL[key]; ok {
		return rel, true
	}
	// strip query
	if i := strings.Index(key, "?"); i >= 0 {
		if rel, ok := idx.ByURL[key[:i]]; ok {
			return rel, true
		}
	}
	base := strings.TrimRight(strings.ToLower(idx.BaseURL), "/")
	key2 := key
	for _, prefix := range []string{base, "https://www.olehdtv.com", "http://www.olehdtv.com"} {
		key2 = strings.TrimPrefix(key2, prefix)
	}
	key2 = strings.TrimPrefix(key2, "/")
	if rel, ok := idx.ByURL[normalizeVisitKey(idx.BaseURL+"/"+key2)]; ok {
		return rel, true
	}
	if rel, ok := idx.ByURL[normalizeVisitKey("/"+key2)]; ok {
		return rel, true
	}
	// direct path under root
	candidate := filepath.Join(idx.Root, filepath.FromSlash(key2))
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return key2, true
	}
	if st, err := os.Stat(candidate + ".html"); err == nil && !st.IsDir() {
		return key2 + ".html", true
	}
	if st, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !st.IsDir() {
		return filepath.ToSlash(filepath.Join(key2, "index.html")), true
	}
	return "", false
}

// SiteOneFetcher resolves pages through ArchiveIndex (SiteOne offline export + fixtures).
type SiteOneFetcher struct {
	Index *ArchiveIndex
}

func NewSiteOneFetcher(root, baseURL string) (*SiteOneFetcher, error) {
	idx, err := BuildArchiveIndex(root, baseURL)
	if err != nil {
		return nil, err
	}
	return &SiteOneFetcher{Index: idx}, nil
}

func (f *SiteOneFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	rel, ok := f.Index.Resolve(pageURL)
	if !ok {
		// fallback to classic FileFetcher mapping for fixtures
		ff := &FileFetcher{Root: f.Index.Root, BaseURL: f.Index.BaseURL}
		return ff.Fetch(ctx, pageURL)
	}
	p := filepath.Join(f.Index.Root, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (f *SiteOneFetcher) FileCount() int {
	if f.Index == nil {
		return 0
	}
	return f.Index.Files
}
