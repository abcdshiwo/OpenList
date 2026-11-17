package common

import (
	"strings"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

// F1: playback (/p/) uses play_proxy_url, download (/d/) uses down_proxy_url.
func TestGeneratePlayProxyURLSeparation(t *testing.T) {
	s := &model.Storage{}
	s.DownProxyURL = "https://sinodl.example.com:6001"
	s.PlayProxyURL = "https://sino.example.com:6001"
	s.DisableProxySign = true

	play := GeneratePlayProxyURL(s, "/下载区/file.mp4")
	if !strings.HasPrefix(play, "https://sino.example.com:6001/") {
		t.Fatalf("play URL should use play proxy, got %s", play)
	}
	down := GenerateDownProxyURL(s, "/下载区/file.mp4")
	if !strings.HasPrefix(down, "https://sinodl.example.com:6001/") {
		t.Fatalf("down URL should use down proxy, got %s", down)
	}
	if play == down {
		t.Fatal("play and down URLs must differ when both configured")
	}
}

// Empty play_proxy_url returns "" so the handler falls back to the download
// proxy (backward compatible: no behavior change until the field is set).
func TestPlayProxyFallbackWhenUnset(t *testing.T) {
	s := &model.Storage{}
	s.DownProxyURL = "https://sinodl.example.com:6001"
	s.DisableProxySign = true
	if got := GeneratePlayProxyURL(s, "/a/b.mp4"); got != "" {
		t.Fatalf("expected empty play URL when unset, got %s", got)
	}
	s2 := &model.Storage{}
	if GenerateDownProxyURL(s2, "/x") != "" || GeneratePlayProxyURL(s2, "/x") != "" {
		t.Fatal("expected empty URLs when nothing configured")
	}
}

func TestProxyURLMultilineAndSign(t *testing.T) {
	s := &model.Storage{}
	s.PlayProxyURL = "https://sino1.example.com:6001\nhttps://sino2.example.com:6001"
	s.DisableProxySign = true
	got := GeneratePlayProxyURL(s, "/a b.mp4")
	if !strings.HasPrefix(got, "https://sino1.example.com:6001/") {
		t.Fatalf("should use first line, got %s", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatal("newline leaked into URL")
	}
	// NOTE: the sign-enabled path shares generateProxyURL with the existing
	// download proxy (verified live); it needs the settings system initialized,
	// so it is covered by integration verification instead of this unit test.
}
