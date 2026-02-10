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
	rawPath := c.Request.URL.Path
	if len(rawPath) < 2 {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, indexHTML)
		return
	}

	// Remove leading "/"
	u := rawPath[1:]

	// Add protocol if missing
	if !strings.HasPrefix(u, "http") {
		u = "https://" + u
	}
	// Fix double-slash stripping by reverse proxies (e.g. nginx / uwsgi)
	if strings.Contains(u[3:9], "://") {
		u = strings.Replace(u, "s:/", "s://", 1)
	}

	// Check URL against patterns
	if !matchURL(u) {
		c.String(http.StatusForbidden, "Invalid input.")
		return
	}

	// blob -> raw conversion
	if exp2.MatchString(u) {
		u = strings.Replace(u, "/blob/", "/raw/", 1)
	}

	// Append query string if present
	fullURL := u
	if c.Request.URL.RawQuery != "" {
		fullURL = u + "?" + c.Request.URL.RawQuery
	}

	doProxy(c, fullURL, false)
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

	// Handle redirect responses: rewrite Location if it points to GitHub
	if location := resp.Header.Get("Location"); location != "" {
		if matchURL(location) {
			// Rewrite to go through our proxy
			resp.Header.Set("Location", "/"+location)
		} else {
			// Follow the redirect internally
			doProxy(c, location, true)
			return
		}
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

func matchURL(u string) bool {
	for _, exp := range exps {
		if exp.MatchString(u) {
			return true
		}
	}
	return false
}
