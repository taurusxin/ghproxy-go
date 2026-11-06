package server

import (
	"context"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Use a real Git client and Git's HTTP backend to check the complete clone
// exchange without requiring GitHub or network access outside localhost.
func TestCloneThroughProxy(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	backend := filepath.Join(runGit("--exec-path"), "git-http-backend")
	if _, err := exec.LookPath(backend); err != nil {
		t.Skip("Git HTTP backend is not installed")
	}
	source := filepath.Join(root, "source")
	bare := filepath.Join(root, "user", "repo.git")
	runGit("init", "--initial-branch=main", source)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("cloned through the proxy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("-C", source, "add", "README.md")
	runGit("-C", source, "-c", "core.hooksPath=", "-c", "commit.gpgsign=false", "-c", "user.name=Proxy Test", "-c", "user.email=proxy@example.test", "commit", "-m", "initial")
	runGit("-C", source, "clone", "--bare", "--local", ".", filepath.ToSlash(bare))
	upstream := httptest.NewServer(&cgi.Handler{
		Path: backend,
		Root: root,
		Env:  []string{"GIT_PROJECT_ROOT=" + filepath.ToSlash(root), "GIT_HTTP_EXPORT_ALL=1"},
	})
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	previous := http.DefaultTransport
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	defer func() { http.DefaultTransport = previous }()
	var uploads atomic.Int32
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "github.com" {
			t.Errorf("unexpected upstream: %s", req.URL)
		}
		if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/git-upload-pack") {
			uploads.Add(1)
		}
		local := req.Clone(req.Context())
		local.URL.Scheme = upstreamURL.Scheme
		local.URL.Host = upstreamURL.Host
		return transport.RoundTrip(local)
	})
	proxy := httptest.NewServer(New("homepage"))
	defer proxy.Close()
	for _, path := range []string{"/user/repo.git", "/user/repo", "/https://github.com/user/repo.git"} {
		t.Run(path, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "clone")
			runGit("-c", "http.proxy=", "clone", "--depth=1", proxy.URL+path, destination)
			content, err := os.ReadFile(filepath.Join(destination, "README.md"))
			if err != nil || string(content) != "cloned through the proxy\n" {
				t.Fatalf("cloned file: %q, %v", content, err)
			}
		})
	}
	if uploads.Load() < 3 {
		t.Fatalf("expected Git upload-pack POSTs, got %d", uploads.Load())
	}
}
