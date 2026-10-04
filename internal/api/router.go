package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kvmymluq2i/cnv-locale-router/internal/store"
)

// NewRouter wires the public HTTP surface. Every error response keeps the
// single top-level error object described in README.md.
func NewRouter(st *store.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		if err := st.Ping(); err != nil {
			respondError(c, http.StatusServiceUnavailable, "storage_unavailable", "database is not available")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "ok"})
	})

	router.POST("/message-entries", postMessageEntry(st))
	router.GET("/route-mappings/:locale", getRouteMapping(st))

	router.NoRoute(func(c *gin.Context) {
		respondError(c, http.StatusNotFound, "route_not_found", "no route matches this path")
	})
	return router
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
