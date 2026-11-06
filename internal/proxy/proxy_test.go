package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newProxyTestContext builds a gin context backed by a response recorder, with a
// GET request suitable for driving doProxy directly in tests.
func newProxyTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

// TestDoProxyFollowsRedirectChain verifies that doProxy follows a redirect chain
// internally and streams the final response body to the client.
func TestDoProxyFollowsRedirectChain(t *testing.T) {
	const body = "FILE_CONTENT"
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer final.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer origin.Close()

	c, w := newProxyTestContext()
	doProxy(c, origin.URL)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != body {
		t.Errorf("body = %q, want %q", w.Body.String(), body)
	}
}

func TestDoProxyRelativeRedirectAndLoop(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/file?download=1", http.StatusFound)
		case "/file":
			if r.URL.RawQuery != "download=1" {
				t.Errorf("query lost: %s", r.URL)
			}
			_, _ = w.Write([]byte("file"))
		default:
			http.Redirect(w, r, "/loop", http.StatusFound)
		}
	}))
	defer origin.Close()
	c, w := newProxyTestContext()
	doProxy(c, origin.URL+"/start")
	if w.Code != http.StatusOK || w.Body.String() != "file" {
		t.Fatalf("response: %d %s", w.Code, w.Body.String())
	}
	c, w = newProxyTestContext()
	doProxy(c, origin.URL+"/loop")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("redirect loop status: %d", w.Code)
	}
}
