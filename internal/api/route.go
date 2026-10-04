package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kvmymluq2i/cnv-locale-router/internal/locale"
	"github.com/kvmymluq2i/cnv-locale-router/internal/store"
)

func getRouteMapping(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		// The locale path parameter follows the same rule as registration and
		// is validated before the namespace query parameter.
		normalizedLocale, ok := locale.Normalize(c.Param("locale"))
		if !ok {
			respondError(c, http.StatusBadRequest, codeInvalidLocale, "locale must be a well-formed BCP 47 language tag")
			return
		}

		values, present := c.Request.URL.Query()["namespace"]
		if !present || len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			respondError(c, http.StatusBadRequest, "InvalidQueryInputError", "query parameter namespace is required exactly once and must not be blank")
			return
		}
		namespace := values[0]

		entry, found, err := st.Route(namespace, normalizedLocale)
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "storage_unavailable", "database is not available")
			return
		}
		if !found {
			respondError(c, http.StatusNotFound, "LocaleNotFoundError", "no message entry matches the namespace and locale")
			return
		}

		// content is deliberately omitted from route responses.
		c.JSON(http.StatusOK, gin.H{
			"namespace": entry.Namespace,
			"locale":    entry.Locale,
			"region":    entry.Region,
			"prefix":    entry.Prefix,
		})
	}
}
