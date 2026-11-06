package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the actual incoming routes, including Git's POST body and escaped
// file paths, without needing GitHub connectivity.
func TestProxyRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New("test homepage")
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	for _, tc := range []struct{ method, path, body, target string }{
		{http.MethodGet, "/user/repo.git/info/refs?service=git-upload-pack", "", "https://github.com/user/repo.git/info/refs?service=git-upload-pack"},
		{http.MethodPost, "/user/repo.git/git-upload-pack", "git request body", "https://github.com/user/repo.git/git-upload-pack"},
		{http.MethodHead, "/user/repo/releases/download/v1/file.zip", "", "https://github.com/user/repo/releases/download/v1/file.zip"},
		{http.MethodGet, "/user/repo/blob/main/a%20b%23c%3Fd.txt?raw=1", "", "https://github.com/user/repo/raw/main/a%20b%23c%3Fd.txt?raw=1"},
		{http.MethodGet, "/https://raw.githubusercontent.com/user/repo/main/file?token=abc", "", "https://raw.githubusercontent.com/user/repo/main/file?token=abc"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			called := false
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				called = true
				if req.URL.String() != tc.target || req.Method != tc.method {
					t.Errorf("upstream: %s %s", req.Method, req.URL)
				}
				body, _ := io.ReadAll(req.Body)
				if string(body) != tc.body {
					t.Errorf("body = %q", body)
				}
				if req.Header.Get("Git-Protocol") != "version=2" {
					t.Error("Git protocol header lost")
				}
				if req.Header.Get("Range") != "bytes=0-99" {
					t.Error("Range header lost")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader("upstream response"))}, nil
			})
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Git-Protocol", "version=2")
			req.Header.Set("Range", "bytes=0-99")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if !called || w.Code != http.StatusOK || w.Body.String() != "upstream response" {
				t.Fatalf("proxy response: %d %s (upstream called: %v)", w.Code, w.Body.String(), called)
			}
		})
	}
}
