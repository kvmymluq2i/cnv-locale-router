package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/language"

	"github.com/kvmymluq2i/cnv-locale-router/internal/store"
)

// Error codes published by the service contract.
const (
	codeStorageUnavailable = "storage_unavailable"
	codeRouteNotFound      = "route_not_found"
	codeInvalidEntryInput  = "InvalidMessageEntryInputError"
	codeInvalidLocale      = "InvalidLocaleInputError"
	codeLocaleConflict     = "LocaleConflictError"
	codeInvalidQuery       = "InvalidQueryInputError"
	codeLocaleNotFound     = "LocaleNotFoundError"
)

// NewRouter wires the public HTTP surface. The service contract in README.md
// describes the error shape every entry must keep.
func NewRouter(st *store.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		if err := st.Ping(); err != nil {
			writeError(c, http.StatusServiceUnavailable, codeStorageUnavailable, "database is not available")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "ok"})
	})

	router.POST("/message-entries", func(c *gin.Context) { createMessageEntry(c, st) })
	router.GET("/route-mappings/:locale", func(c *gin.Context) { getRouteMapping(c, st) })

	router.NoRoute(func(c *gin.Context) {
		writeError(c, http.StatusNotFound, codeRouteNotFound, "no route matches this path")
	})
	return router
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// createMessageEntry handles POST /message-entries.
func createMessageEntry(c *gin.Context, st *store.Store) {
	fields, ok := decodeEntryObject(c)
	if !ok {
		return
	}

	namespace := fields["namespace"]
	if strings.TrimSpace(namespace) == "" {
		writeError(c, http.StatusBadRequest, codeInvalidEntryInput, "namespace must not be blank")
		return
	}
	content := fields["content"]
	if content == "" {
		writeError(c, http.StatusBadRequest, codeInvalidEntryInput, "content must not be empty")
		return
	}
	region, ok := normalizeRegion(fields["region"])
	if !ok {
		writeError(c, http.StatusBadRequest, codeInvalidEntryInput, "region must be exactly two ASCII letters")
		return
	}
	prefix := fields["prefix"]
	if !validPrefix(prefix) {
		writeError(c, http.StatusBadRequest, codeInvalidEntryInput, "prefix must start with / and contain no whitespace, ? or #")
		return
	}

	locale, ok := normalizeLocale(fields["locale"])
	if !ok {
		writeError(c, http.StatusBadRequest, codeInvalidLocale, "locale must be a valid BCP 47 language tag")
		return
	}

	entry := store.MessageEntry{
		Namespace: namespace,
		Locale:    locale,
		Region:    region,
		Prefix:    prefix,
		Content:   content,
	}
	saved, _, err := st.SaveMessageEntry(entry)
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(c, http.StatusConflict, codeLocaleConflict, "entry conflicts with an existing registration")
		return
	case err != nil:
		writeError(c, http.StatusServiceUnavailable, codeStorageUnavailable, "database is not available")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"namespace": saved.Namespace,
		"locale":    saved.Locale,
		"region":    saved.Region,
		"prefix":    saved.Prefix,
		"content":   saved.Content,
	})
}

// decodeEntryObject reads exactly one JSON object from the request body and
// extracts the five required string fields.
func decodeEntryObject(c *gin.Context) (map[string]string, bool) {
	invalid := func() (map[string]string, bool) {
		writeError(c, http.StatusBadRequest, codeInvalidEntryInput, "body must be a single JSON object with string fields namespace, locale, region, prefix and content")
		return nil, false
	}

	decoder := json.NewDecoder(c.Request.Body)
	var payload interface{}
	if err := decoder.Decode(&payload); err != nil {
		return invalid()
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return invalid()
	}
	object, ok := payload.(map[string]interface{})
	if !ok {
		return invalid()
	}

	fields := make(map[string]string, 5)
	for _, name := range []string{"namespace", "locale", "region", "prefix", "content"} {
		raw, present := object[name]
		if !present {
			return invalid()
		}
		value, ok := raw.(string)
		if !ok {
			return invalid()
		}
		fields[name] = value
	}
	return fields, true
}

// getRouteMapping handles GET /route-mappings/:locale?namespace=...
func getRouteMapping(c *gin.Context, st *store.Store) {
	locale, ok := normalizeLocale(c.Param("locale"))
	if !ok {
		writeError(c, http.StatusBadRequest, codeInvalidLocale, "locale must be a valid BCP 47 language tag")
		return
	}

	namespaces := c.Request.URL.Query()["namespace"]
	if len(namespaces) != 1 || strings.TrimSpace(namespaces[0]) == "" {
		writeError(c, http.StatusBadRequest, codeInvalidQuery, "exactly one non-blank namespace query parameter is required")
		return
	}

	entry, err := st.FindMessageEntry(namespaces[0], locale)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(c, http.StatusNotFound, codeLocaleNotFound, "no message entry matches this namespace and locale")
		return
	case err != nil:
		writeError(c, http.StatusServiceUnavailable, codeStorageUnavailable, "database is not available")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"namespace": entry.Namespace,
		"locale":    entry.Locale,
		"region":    entry.Region,
		"prefix":    entry.Prefix,
	})
}

// normalizeLocale validates a BCP 47 language tag and returns it with
// canonical casing (lowercase language, titlecase script, uppercase region).
func normalizeLocale(raw string) (string, bool) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "", false
	}
	// language.Parse is lenient about separators, so enforce the BCP 47
	// skeleton first: hyphen-separated subtags of 1-8 ASCII alphanumerics.
	for _, subtag := range strings.Split(raw, "-") {
		if len(subtag) < 1 || len(subtag) > 8 {
			return "", false
		}
		for _, r := range subtag {
			if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9') {
				return "", false
			}
		}
	}
	tag, err := language.Parse(raw)
	if err != nil {
		return "", false
	}
	return tag.String(), true
}

// normalizeRegion uppercases a region code, requiring exactly two ASCII letters.
func normalizeRegion(raw string) (string, bool) {
	if len(raw) != 2 {
		return "", false
	}
	for _, r := range raw {
		if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z') {
			return "", false
		}
	}
	return strings.ToUpper(raw), true
}

// validPrefix reports whether prefix starts with / and carries no whitespace,
// ? or #. Bare / is allowed.
func validPrefix(prefix string) bool {
	if !strings.HasPrefix(prefix, "/") {
		return false
	}
	for _, r := range prefix {
		if unicode.IsSpace(r) || r == '?' || r == '#' {
			return false
		}
	}
	return true
}
