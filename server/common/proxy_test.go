package common

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
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
}

func TestProxyOverridesUpstreamContentDisposition(t *testing.T) {
	previousConfig := conf.Conf
	conf.Conf = conf.DefaultConfig("data")
	t.Cleanup(func() {
		conf.Conf = previousConfig
	})

	const content = "archive content"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="download"`)
		w.Header().Set("Content-Type", "application/x-rar-compressed")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, content)
	}))
	t.Cleanup(upstream.Close)

	file := &model.Object{
		Name: "测试文件.rar",
		Size: int64(len(content)),
	}
	link := &model.Link{URL: upstream.URL}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sd/example", nil)

	err := Proxy(recorder, request, link, file)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status code = %d, want %d", got, want)
	}
	if got, want := recorder.Header().Get("Content-Disposition"), utils.GenerateContentDisposition(file.GetName()); got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if got, want := recorder.Header().Get("Content-Type"), "application/x-rar-compressed"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got, want := recorder.Body.String(), content; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
