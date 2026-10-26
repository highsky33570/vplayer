package mediasign

import (
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/media"
)

const testSecret = "test-media-signing-secret"

func TestSignVerify(t *testing.T) {
	key := "videos/9/8/index.m3u8"
	exp := time.Now().Add(10 * time.Minute).Unix()
	sig, err := Sign(testSecret, key, exp)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(testSecret, key, sig, exp, time.Now().Unix(), 600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(testSecret, key, "", exp, time.Now().Unix(), 600); err == nil {
		t.Fatal("missing signature")
	}
	if err := Verify(testSecret, key, "zz", exp, time.Now().Unix(), 600); err == nil {
		t.Fatal("malformed signature")
	}
	if err := Verify(testSecret, key, strings.Repeat("ab", 32), exp, time.Now().Unix(), 600); err == nil {
		t.Fatal("wrong signature")
	}
	if err := Verify(testSecret, key, sig, exp-3600, time.Now().Unix(), 600); err == nil {
		t.Fatal("expired")
	}
	if err := Verify(testSecret, key, sig, exp+1, time.Now().Unix(), 600); err == nil {
		t.Fatal("modified expiration")
	}
	if err := Verify(testSecret, "videos/1/1/index.m3u8", sig, exp, time.Now().Unix(), 600); err == nil {
		t.Fatal("modified path")
	}
	if err := Verify(testSecret, key, sig, time.Now().Add(24*time.Hour).Unix(), time.Now().Unix(), 600); err == nil {
		t.Fatal("ttl beyond max")
	}
}

func TestCanonicalRejectsTraversal(t *testing.T) {
	for _, raw := range []string{
		"../secret",
		"videos/../secret",
		"videos/1/../../etc/passwd",
		"videos/%2e%2e/secret",
		"videos/1/%2fsecret",
		"/etc/passwd",
		"posters/a.jpg",
		"videos/1/2/../../x",
		"videos\\1\\2",
		"",
	} {
		if _, err := CanonicalObjectKey(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if _, err := CanonicalObjectKey("videos/9/8/index0.ts"); err != nil {
		t.Fatal(err)
	}
}

func TestRewritePlaylist(t *testing.T) {
	opts := media.CDNURLOptions{BaseURL: "https://cdn.example", Bucket: "dongman", PathMode: media.CDNPathModeBucketPrefix}
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-STREAM-INF:BANDWIDTH=800000",
		"720/index.m3u8",
		"#EXT-X-MAP:URI=\"init.mp4\"",
		"#EXT-X-KEY:METHOD=AES-128,URI=\"enc.key\",IV=0x01",
		"#EXTINF:6.0,",
		"seg-1.ts",
		"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",NAME=\"a\",URI=\"audio/index.m3u8\"",
		"https://cdn.example/dongman/videos/9/8/from-cdn.m4s",
	}, "\n") + "\n"
	out, err := RewritePlaylist(body, "videos/9/8/index.m3u8", opts, func(objectKey string) (string, error) {
		return "https://play.example/api/v1/media/" + objectKey + "?exp=1&sig=ab", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https://play.example/api/v1/media/videos/9/8/720/index.m3u8?exp=1&sig=ab",
		`URI="https://play.example/api/v1/media/videos/9/8/init.mp4?exp=1&sig=ab"`,
		`URI="https://play.example/api/v1/media/videos/9/8/enc.key?exp=1&sig=ab"`,
		"https://play.example/api/v1/media/videos/9/8/seg-1.ts?exp=1&sig=ab",
		"https://play.example/api/v1/media/videos/9/8/audio/index.m3u8?exp=1&sig=ab",
		"https://play.example/api/v1/media/videos/9/8/from-cdn.m4s?exp=1&sig=ab",
		"#EXT-X-STREAM-INF:BANDWIDTH=800000",
		"IV=0x01",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s\n%s", want, out)
		}
	}
	if strings.Contains(out, "cdn.example") {
		t.Fatalf("durable cdn url remained:\n%s", out)
	}
}

func TestRewriteRejectsExternalAndTraversal(t *testing.T) {
	opts := media.CDNURLOptions{BaseURL: "https://cdn.example", Bucket: "dongman"}
	sign := func(objectKey string) (string, error) {
		t.Fatalf("should not sign %s", objectKey)
		return "", nil
	}
	for _, body := range []string{
		"https://evil.example/secret.ts\n",
		"http://127.0.0.1/latest/meta-data\n",
		"../../secret.ts\n",
		"#EXT-X-KEY:METHOD=AES-128,URI=\"https://evil.example/key\"\n",
	} {
		if _, err := RewritePlaylist("#EXTM3U\n"+body, "videos/9/8/index.m3u8", opts, sign); err == nil {
			t.Fatalf("accepted\n%s", body)
		}
	}
}

func TestSignedURLRoundTripPath(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	raw, err := SignedMediaURL("https://play.example", "videos/9/8/seg 1.ts", testSecret, 600, now)
	if err == nil {
		t.Fatalf("space in key should fail, got %s", raw)
	}
	raw, err = SignedMediaURL("https://play.example", "videos/9/8/seg-1.ts", testSecret, 600, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "/api/v1/media/videos/9/8/seg-1.ts?exp=") {
		t.Fatal(raw)
	}
	exp := now.Add(600 * time.Second).Unix()
	sig, _ := Sign(testSecret, "videos/9/8/seg-1.ts", exp)
	if !strings.Contains(raw, sig) {
		t.Fatal(raw)
	}
}
