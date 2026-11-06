// Package proxy resolves supported GitHub resources and forwards their HTTP traffic.
package proxy

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	// Repository roots, including clone URLs ending in .git.
	repositoryURL = regexp.MustCompile(`^(?:https?://)?github\.com/[^/?#]+/[^/?#]+/?$`)
	// releases / archive: github.com/user/repo/releases/... or github.com/user/repo/archive/...
	downloadURL = regexp.MustCompile(`^(?:https?://)?github\.com/.+?/.+?/(?:releases|archive)/.*$`)
	// blob / raw files: github.com/user/repo/blob/... or github.com/user/repo/raw/...
	fileURL = regexp.MustCompile(`^(?:https?://)?github\.com/.+?/.+?/(?:blob|raw)/.*$`)
	// git clone: github.com/user/repo/info/... or github.com/user/repo/git-...
	gitURL = regexp.MustCompile(`^(?:https?://)?github\.com/.+?/.+?/(?:info|git-).*$`)
	// raw.githubusercontent.com / raw.github.com
	rawURL = regexp.MustCompile(`^(?:https?://)?raw\.(?:githubusercontent|github)\.com/.+?/.+?/.+?/.+$`)
	// gist files
	gistURL = regexp.MustCompile(`^(?:https?://)?gist\.(?:githubusercontent|github)\.com/.+?/.+?/.+$`)

	allowedURLs = []*regexp.Regexp{repositoryURL, downloadURL, fileURL, gitURL, rawURL, gistURL}

	// githubHosts are the upstream hosts recognised in full-form proxy URLs.
	githubHosts = []string{
		"github.com/",
		"raw.githubusercontent.com/",
		"raw.github.com/",
		"gist.githubusercontent.com/",
		"gist.github.com/",
	}
)

// resolveTarget converts an incoming proxy path into the upstream GitHub URL to
// fetch. It returns ok=false for the index page (empty path) or for requests
// that do not match an allowed pattern.
//
// Two URL forms are supported:
//
//	full form:  https://example.com/https://github.com/user/repo   (any GitHub resource)
//	short form: https://example.com/user/repo                       (github.com resources)
//
// The short form replaces "github.com" with the proxy domain, so the path
// carries no explicit host. Both forms support the same GitHub resources.
func resolveTarget(rawPath, rawQuery string) (string, bool) {
	if len(rawPath) < 2 {
		return "", false
	}

	// Remove leading "/"
	u := rawPath[1:]

	if !isFullGitHubURL(u) {
		// Treat the path as relative to github.com.
		u = "https://github.com/" + u
	} else {
		// Add protocol if missing
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") &&
			!strings.HasPrefix(u, "http:/") && !strings.HasPrefix(u, "https:/") {
			u = "https://" + u
		}
		// Repair schemes when a reverse proxy collapses consecutive slashes.
		if strings.HasPrefix(u, "https:/") && !strings.HasPrefix(u, "https://") ||
			strings.HasPrefix(u, "http:/") && !strings.HasPrefix(u, "http://") {
			u = strings.Replace(u, ":/", "://", 1)
		}
	}

	// Parse before matching so malformed URLs cannot become proxy targets.
	target, err := url.Parse(u)
	if err != nil || target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
		!matchURL(u) {
		return "", false
	}
	for _, part := range strings.Split(target.Path, "/") {
		if part == "." || part == ".." {
			return "", false
		}
	}

	// blob -> raw conversion
	if fileURL.MatchString(u) {
		parts := strings.SplitN(target.EscapedPath(), "/", 5)
		if len(parts) == 5 && parts[3] == "blob" {
			parts[3] = "raw"
			u = target.Scheme + "://" + target.Host + strings.Join(parts, "/")
		}
	}

	// Append query string if present
	if rawQuery != "" {
		u = u + "?" + rawQuery
	}
	return u, true
}

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
	for _, pattern := range allowedURLs {
		if pattern.MatchString(u) {
			return true
		}
	}
	return false
}
