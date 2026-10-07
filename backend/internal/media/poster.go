package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

type PosterResult struct {
	Key         string
	ContentType string
	Skipped     bool
	Bytes       int
}

type PosterSyncer struct {
	Store      ObjectStore
	HTTP       *http.Client
	FetchEnabled bool
	MaxBytes   int64
}

func NewPosterSyncer(store ObjectStore, fetchEnabled bool) *PosterSyncer {
	return &PosterSyncer{
		Store:        store,
		FetchEnabled: fetchEnabled,
		MaxBytes:     8 << 20, // 8 MiB
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func PosterKey(sourceSystem, sourceID, posterURL string) string {
	sum := sha256.Sum256([]byte(sourceSystem + "|" + sourceID + "|" + posterURL))
	ext := extFromURL(posterURL)
	return fmt.Sprintf("posters/%s/%s%s", sourceSystem, hex.EncodeToString(sum[:16]), ext)
}

func extFromURL(u string) string {
	clean := u
	if i := strings.Index(clean, "?"); i >= 0 {
		clean = clean[:i]
	}
	ext := strings.ToLower(path.Ext(clean))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return ext
	default:
		return ".jpg"
	}
}

func (p *PosterSyncer) Sync(ctx context.Context, sourceSystem, sourceID, posterURL string) (*PosterResult, error) {
	if posterURL == "" {
		return nil, fmt.Errorf("empty poster url")
	}
	key := PosterKey(sourceSystem, sourceID, posterURL)
	exists, err := p.Store.Exists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return &PosterResult{Key: key, Skipped: true}, nil
	}
	if !p.FetchEnabled {
		return nil, fmt.Errorf("poster fetch disabled (set OLEHDTV_FETCH_POSTERS=true)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, posterURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VPlayerPosterSync/1.0")
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poster http %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") {
		return nil, fmt.Errorf("unexpected content-type %q", ct)
	}
	limited := io.LimitReader(resp.Body, p.MaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > p.MaxBytes {
		return nil, fmt.Errorf("poster too large")
	}
	if ct == "" {
		ct = "image/jpeg"
	}
	if err := p.Store.Put(ctx, key, body, ct); err != nil {
		return nil, err
	}
	return &PosterResult{Key: key, ContentType: ct, Bytes: len(body)}, nil
}
