package httpapi

import "testing"

func TestValidateAdminPlaybackURL(t *testing.T) {
	if _, msg := validateAdminPlaybackURL(""); msg == "" {
		t.Fatal("empty must fail")
	}
	if _, msg := validateAdminPlaybackURL("not-a-url"); msg == "" {
		t.Fatal("relative must fail")
	}
	if u, msg := validateAdminPlaybackURL("https://cdn.example/a.m3u8"); msg != "" || u == "" {
		t.Fatalf("https ok: %q %q", u, msg)
	}
	if src := playbackSourceForURL("https://cdn.example/a.m3u8"); src != "m3u8" {
		t.Fatalf("source=%q", src)
	}
	if src := playbackSourceForURL("https://cdn.example/a.mp4"); src != "direct" {
		t.Fatalf("source=%q", src)
	}
}

func TestAdminCreateAsVIPDefault(t *testing.T) {
	if adminCreateAsVIP(adminCreateVideoReq{}) {
		t.Fatal("default as_vip must be false; category selection is required")
	}
	f := false
	if adminCreateAsVIP(adminCreateVideoReq{AsVIP: &f}) {
		t.Fatal("explicit false")
	}
	tr := true
	if !adminCreateAsVIP(adminCreateVideoReq{AsVIP: &tr}) {
		t.Fatal("explicit true")
	}
}
