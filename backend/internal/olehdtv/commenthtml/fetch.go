package commenthtml

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

// Client loads one public OLEHDTV comment page. It only issues GET comment/ajax.html.
type Client struct {
	BaseURL string
	HTTP    *maccms.HTTPFetcher
}

var ridPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

// NewClient uses the catalog fetcher limits: user agent, timeout, delay, retries, and the 4MB cap.
func NewClient(baseURL string, timeout time.Duration, delayMs, retries int) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://www.olehdtv.com"
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    maccms.NewHTTPFetcher(timeout, delayMs, retries, "VPlayerCatalogSync/1.0"),
	}
}

// ListURL is the anonymous comment list the public page requests.
func ListURL(baseURL, sourceVideoID string, page int) (string, error) {
	if page < 1 {
		return "", fmt.Errorf("invalid comment page")
	}
	if !ridPattern.MatchString(strings.TrimSpace(sourceVideoID)) {
		return "", fmt.Errorf("invalid source video id")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://www.olehdtv.com"
	}
	u, err := url.Parse(baseURL + "/index.php/comment/ajax.html")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("rid", sourceVideoID)
	q.Set("mid", "1")
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	if err := refuseNonRead(u.String()); err != nil {
		return "", err
	}
	return u.String(), nil
}

// FetchCommentPage performs one anonymous GET and returns HTML.
// An application/json string body is decoded. Raw HTML is returned as-is.
func (c *Client) FetchCommentPage(ctx context.Context, sourceVideoID string, page int) (string, error) {
	if c == nil || c.HTTP == nil {
		return "", fmt.Errorf("comment client is not configured")
	}
	pageURL, err := ListURL(c.BaseURL, sourceVideoID, page)
	if err != nil {
		return "", err
	}
	raw, err := c.HTTP.FetchHeaders(ctx, pageURL, map[string]string{
		"Accept":           "application/json, text/javascript, */*; q=0.01",
		"X-Requested-With": "XMLHttpRequest",
	})
	if err != nil {
		return "", err
	}
	return DecodePayload(raw)
}

func refuseNonRead(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Path != "/index.php/comment/ajax.html" {
		return fmt.Errorf("refusing non-read comment url")
	}
	lower := strings.ToLower(u.String())
	for _, banned := range []string{"savedata", "gbook", "verify", "digg", "report"} {
		if strings.Contains(lower, banned) {
			return fmt.Errorf("refusing non-read comment url")
		}
	}
	return nil
}
