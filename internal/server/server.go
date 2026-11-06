// Package server assembles the HTTP routes and homepage.
package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/taurusxin/ghproxy-go/internal/proxy"
)

func New(indexHTML string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		if q := c.Query("q"); q != "" {
			// Always keep this redirect on the proxy's own origin.
			c.Redirect(http.StatusFound, "/"+strings.TrimLeft(q, "/\\"))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(indexHTML))
	})
	router.NoRoute(proxy.Handler)
	return router
}
