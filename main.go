package main

import (
	_ "embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"os"

	"github.com/gin-gonic/gin"
)

//go:embed templates/index.html
var indexHTML string

// ======================== Configuration ========================

const (
	defaultHost = "0.0.0.0"
	defaultPort = "8972"
	sizeLimit   = 1024 * 1024 * 1024 * 999 // 999GB
	chunkSize   = 1024 * 10                // 10KB
)

// ======================== URL Patterns ========================

var (
	// releases / archive: github.com/user/repo/releases/... or github.com/user/repo/archive/...
	exp1 = regexp.MustCompile(`^(?:https?://)?github\.com/.+?/.+?/(?:releases|archive)/.*$`)
	// blob / raw files: github.com/user/repo/blob/... or github.com/user/repo/raw/...
	exp2 = regexp.MustCompile(`^(?:https?://)?github\.com/.+?/.+?/(?:blob|raw)/.*$`)
	// git clone: github.com/user/repo/info/... or github.com/user/repo/git-...
	exp3 = regexp.MustCompile(`^(?:https?://)?github\.com/.+?/.+?/(?:info|git-).*$`)
	// raw.githubusercontent.com / raw.github.com
	exp4 = regexp.MustCompile(`^(?:https?://)?raw\.(?:githubusercontent|github)\.com/.+?/.+?/.+?/.+$`)
	// gist files
	exp5 = regexp.MustCompile(`^(?:https?://)?gist\.(?:githubusercontent|github)\.com/.+?/.+?/.+$`)

	exps = []*regexp.Regexp{exp1, exp2, exp3, exp4, exp5}

	// githubHosts are the upstream hosts recognised in full-form proxy URLs.
	githubHosts = []string{
		"github.com/",
		"raw.githubusercontent.com/",
		"raw.github.com/",
		"gist.githubusercontent.com/",
		"gist.github.com/",
	}
)

// ======================== Main ========================

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.GET("/", indexHandler)
	r.NoRoute(proxyHandler)

	host := os.Getenv("HOST")
	if host == "" {
		host = defaultHost
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	log.Printf("GitHub Proxy is running on http://%s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// ======================== Handlers ========================

func indexHandler(c *gin.Context) {
	// If ?q= parameter exists, redirect to the proxy path
	if q := c.Query("q"); q != "" {
		c.Redirect(http.StatusFound, "/"+q)
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, indexHTML)
}

func proxyHandler(c *gin.Context) {
	fullURL, ok := resolveTarget(c.Request.URL.Path, c.Request.URL.RawQuery)
	if !ok {
		if len(c.Request.URL.Path) < 2 {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, indexHTML)
			return
		}
		c.String(http.StatusForbidden, "Invalid input.")
		return
	}
	doProxy(c, fullURL, false)
}

// resolveTarget converts an incoming proxy path into the upstream GitHub URL to
// fetch. It returns ok=false for the index page (empty path) or for requests
// that do not match an allowed pattern.
//
// Two URL forms are supported:
//
//	full form:  https://example.com/https://github.com/user/repo   (any GitHub resource)
//	short form: https://example.com/user/repo                       (git clone & release/archive only)
//
// The short form replaces "github.com" with the proxy domain, so the path
// carries no explicit host. It is restricted to git clone (info/git- endpoints)
// and release/archive downloads; accessing any web resource is rejected.
func resolveTarget(rawPath, rawQuery string) (string, bool) {
	if len(rawPath) < 2 {
		return "", false
	}

	// Remove leading "/"
	u := rawPath[1:]

	shortForm := !isFullGitHubURL(u)
	if shortForm {
		// Treat the path as relative to github.com.
		u = "https://github.com/" + u
	} else {
		// Add protocol if missing
		if !strings.HasPrefix(u, "http") {
			u = "https://" + u
		}
		// Fix double-slash stripping by reverse proxies (e.g. nginx / uwsgi)
		if !strings.Contains(u[3:9], "://") {
			u = strings.Replace(u, "s:/", "s://", 1)
		}
	}

	// Check URL against patterns
	if shortForm {
		if !exp1.MatchString(u) && !exp3.MatchString(u) {
			return "", false
		}
	} else if !matchURL(u) {
		return "", false
	}

	// blob -> raw conversion
	if exp2.MatchString(u) {
		u = strings.Replace(u, "/blob/", "/raw/", 1)
	}

	// Append query string if present
	if rawQuery != "" {
		u = u + "?" + rawQuery
	}
	return u, true
}

// ======================== Core Proxy Logic ========================

func doProxy(c *gin.Context, targetURL string, allowRedirects bool) {
	// Build upstream request
	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create request: %v", err)
		return
	}

	// Copy headers from the original request, remove Host
	for key, values := range c.Request.Header {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}

	// Use a client that does NOT follow redirects automatically
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if allowRedirects {
		client = &http.Client{}
	}

	resp, err := client.Do(req)
	if err != nil {
		c.String(http.StatusInternalServerError, "Server error: %v", err)
		return
	}
	defer resp.Body.Close()

	// Check Content-Length against size limit
	if resp.ContentLength > sizeLimit {
		c.Redirect(http.StatusFound, targetURL)
		return
	}

	// Follow redirect responses internally so the client receives the final
	// content directly. This keeps the proxy in the path (important when the
	// client cannot reach GitHub's CDN on its own) and resolves redirect chains
	// such as /releases/latest/download/... -> /releases/download/<ver>/... -> CDN
	// without bouncing a 302 back to the client.
	if location := resp.Header.Get("Location"); location != "" {
		doProxy(c, location, true)
		return
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, v := range values {
			c.Header(key, v)
		}
	}

	// Stream the response body
	c.Status(resp.StatusCode)
	buf := make([]byte, chunkSize)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				log.Printf("Write error: %v", writeErr)
				return
			}
			c.Writer.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				log.Printf("Read error: %v", readErr)
			}
			break
		}
	}
}

// ======================== Helpers ========================

// isFullGitHubURL reports whether the given path (with the leading "/" already
// removed) carries an explicit GitHub host, i.e. the classic full-URL proxy
// form such as https://example.com/https://github.com/user/repo. A path without
// a host (e.g. "user/repo") is the short form and returns false.
func isFullGitHubURL(u string) bool {
	// Strip a scheme prefix, including the single-slash variant produced when a
	// reverse proxy collapses "https://" to "https:/".
	for _, p := range []string{"https://", "http://", "https:/", "http:/"} {
		if strings.HasPrefix(u, p) {
			u = u[len(p):]
			break
		}
	}
	for _, h := range githubHosts {
		if strings.HasPrefix(u, h) {
			return true
		}
	}
	return false
}

func matchURL(u string) bool {
	for _, exp := range exps {
		if exp.MatchString(u) {
			return true
		}
	}
	return false
}
