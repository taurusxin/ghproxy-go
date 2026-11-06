package proxy

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	sizeLimit = 1024 * 1024 * 1024 * 999 // 999GB
	chunkSize = 1024 * 10                // 10KB
)

// Handler resolves supported GitHub URLs and streams upstream responses.
func Handler(c *gin.Context) {
	target, ok := resolveTarget(c.Request.URL.EscapedPath(), c.Request.URL.RawQuery)
	if !ok {
		c.String(http.StatusForbidden, "Invalid input.")
		return
	}
	doProxy(c, target)
}

func doProxy(c *gin.Context, targetURL string) {
	// Build upstream request
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create request: %v", err)
		return
	}
	req.ContentLength = c.Request.ContentLength

	// Copy headers from the original request, remove Host
	for key, values := range c.Request.Header {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}

	// Follow relative and absolute redirects internally, with Go's default
	// ten-redirect limit, so downloads remain on the proxy connection.
	client := &http.Client{}
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
