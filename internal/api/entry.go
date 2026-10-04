package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/kvmymluq2i/cnv-locale-router/internal/locale"
	"github.com/kvmymluq2i/cnv-locale-router/internal/store"
)

const (
	codeInvalidEntry   = "InvalidMessageEntryInputError"
	codeInvalidLocale  = "InvalidLocaleInputError"
	codeLocaleConflict = "LocaleConflictError"
)

// entryInput uses pointers so a missing field and a present empty string can
// be told apart; every field is *string, which also makes a wrong JSON type a
// hard type error.
type entryInput struct {
	Namespace *string `json:"namespace"`
	Locale    *string `json:"locale"`
	Region    *string `json:"region"`
	Prefix    *string `json:"prefix"`
	Content   *string `json:"content"`
}

func postMessageEntry(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := decodeEntryInput(c)
		if !ok {
			return
		}

		// Non-locale fields are validated first; the locale is only checked
		// once the rest of the input is acceptable, and conflict detection
		// comes after every input validation.
		namespace := *input.Namespace
		if strings.TrimSpace(namespace) == "" {
			respondError(c, http.StatusBadRequest, codeInvalidEntry, "namespace must be a non-blank string")
			return
		}
		if *input.Content == "" {
			respondError(c, http.StatusBadRequest, codeInvalidEntry, "content must be a non-empty string")
			return
		}
		region, ok := normalizeRegion(*input.Region)
		if !ok {
			respondError(c, http.StatusBadRequest, codeInvalidEntry, "region must be exactly two ASCII letters")
			return
		}
		prefix := *input.Prefix
		if !validPrefix(prefix) {
			respondError(c, http.StatusBadRequest, codeInvalidEntry, "prefix must start with '/' and contain no whitespace, '?' or '#'")
			return
		}
		normalizedLocale, ok := locale.Normalize(*input.Locale)
		if !ok {
			respondError(c, http.StatusBadRequest, codeInvalidLocale, "locale must be a well-formed BCP 47 language tag")
			return
		}

		entry := store.Entry{
			Namespace: namespace,
			Locale:    normalizedLocale,
			Region:    region,
			Prefix:    prefix,
			Content:   *input.Content,
		}
		if err := st.PutEntry(entry); err != nil {
			if store.IsConflict(err) {
				respondError(c, http.StatusConflict, codeLocaleConflict, "the locale or region conflicts with an existing registration")
				return
			}
			respondError(c, http.StatusServiceUnavailable, "storage_unavailable", "database is not available")
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"namespace": entry.Namespace,
			"locale":    entry.Locale,
			"region":    entry.Region,
			"prefix":    entry.Prefix,
			"content":   entry.Content,
		})
	}
}

// decodeEntryInput requires exactly one JSON object carrying the five string
// fields. Any other shape is InvalidMessageEntryInputError.
func decodeEntryInput(c *gin.Context) (entryInput, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, http.StatusBadRequest, codeInvalidEntry, "request body could not be read")
		return entryInput{}, false
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed[0] != '{' {
		respondError(c, http.StatusBadRequest, codeInvalidEntry, "request body must be a single JSON object")
		return entryInput{}, false
	}

	// A single decode of a single value, after which the stream must be empty:
	// this rejects two objects, arrays, scalars and trailing tokens alike.
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil || len(raw) == 0 {
		respondError(c, http.StatusBadRequest, codeInvalidEntry, "request body must be a single JSON object")
		return entryInput{}, false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		respondError(c, http.StatusBadRequest, codeInvalidEntry, "request body must be a single JSON object")
		return entryInput{}, false
	}

	var input entryInput
	for _, field := range []struct {
		name string
		raw  json.RawMessage
		dst  **string
	}{
		{"namespace", raw["namespace"], &input.Namespace},
		{"locale", raw["locale"], &input.Locale},
		{"region", raw["region"], &input.Region},
		{"prefix", raw["prefix"], &input.Prefix},
		{"content", raw["content"], &input.Content},
	} {
		if field.raw == nil {
			respondError(c, http.StatusBadRequest, codeInvalidEntry, field.name+" is required")
			return entryInput{}, false
		}
		var s string
		if bytes.Equal(field.raw, []byte("null")) || json.Unmarshal(field.raw, &s) != nil {
			respondError(c, http.StatusBadRequest, codeInvalidEntry, field.name+" must be a string")
			return entryInput{}, false
		}
		*field.dst = &s
	}
	return input, true
}

// normalizeRegion accepts exactly two ASCII letters and upper-cases them.
func normalizeRegion(region string) (string, bool) {
	if len(region) != 2 {
		return "", false
	}
	upper := strings.ToUpper(region)
	for i := 0; i < 2; i++ {
		b := upper[i]
		if b < 'A' || b > 'Z' {
			return "", false
		}
	}
	return upper, true
}

// validPrefix requires a leading slash and forbids whitespace, '?' and '#'.
func validPrefix(prefix string) bool {
	if prefix == "" || prefix[0] != '/' {
		return false
	}
	for _, r := range prefix {
		if r == '?' || r == '#' || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
