package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kvmymluq2i/cnv-locale-router/internal/store"
)

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewRouter(st)
}

func serve(router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, reader)
	router.ServeHTTP(recorder, request)
	return recorder
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error body not JSON: %v (%s)", err, recorder.Body.String())
	}
	if payload.Error.Code == "" || payload.Error.Message == "" {
		t.Fatalf("error payload missing fields: %s", recorder.Body.String())
	}
	return payload.Error.Code
}

func TestHealthzReportsOK(t *testing.T) {
	recorder := serve(newTestRouter(t), http.MethodGet, "/healthz", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != `{"database":"ok","status":"ok"}` {
		t.Fatalf("body = %s", got)
	}
}

func TestUnknownRouteUsesPublishedErrorShape(t *testing.T) {
	recorder := serve(newTestRouter(t), http.MethodGet, "/missing", "")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if code := errorCode(t, recorder); code != "route_not_found" {
		t.Fatalf("code = %q", code)
	}
}

const validEntryBody = `{"namespace":"checkout","locale":"EN-us","region":"us","prefix":"/us","content":"Hello"}`

func TestCreateMessageEntryNormalizesAndReturnsSaved(t *testing.T) {
	recorder := serve(newTestRouter(t), http.MethodPost, "/message-entries", validEntryBody)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	want := map[string]string{
		"namespace": "checkout",
		"locale":    "en-US",
		"region":    "US",
		"prefix":    "/us",
		"content":   "Hello",
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("%s = %q, want %q (body %s)", key, payload[key], value, recorder.Body.String())
		}
	}
}

func TestCreateMessageEntryRejectsMalformedBodies(t *testing.T) {
	router := newTestRouter(t)
	bodies := map[string]string{
		"array":           `[{"namespace":"a"}]`,
		"scalar":          `"hello"`,
		"trailing object": `{"namespace":"a","locale":"en","region":"US","prefix":"/a","content":"x"} {}`,
		"empty":           ``,
		"missing field":   `{"namespace":"a","locale":"en","region":"US","prefix":"/a"}`,
		"numeric field":   `{"namespace":"a","locale":"en","region":"US","prefix":"/a","content":3}`,
		"null field":      `{"namespace":"a","locale":"en","region":"US","prefix":"/a","content":null}`,
		"blank namespace": `{"namespace":"   ","locale":"en","region":"US","prefix":"/a","content":"x"}`,
		"empty content":   `{"namespace":"a","locale":"en","region":"US","prefix":"/a","content":""}`,
		"short region":    `{"namespace":"a","locale":"en","region":"U","prefix":"/a","content":"x"}`,
		"long region":     `{"namespace":"a","locale":"en","region":"USA","prefix":"/a","content":"x"}`,
		"digit region":    `{"namespace":"a","locale":"en","region":"1S","prefix":"/a","content":"x"}`,
		"prefix no slash": `{"namespace":"a","locale":"en","region":"US","prefix":"a","content":"x"}`,
		"prefix space":    `{"namespace":"a","locale":"en","region":"US","prefix":"/a b","content":"x"}`,
		"prefix query":    `{"namespace":"a","locale":"en","region":"US","prefix":"/a?b","content":"x"}`,
		"prefix fragment": `{"namespace":"a","locale":"en","region":"US","prefix":"/a#b","content":"x"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			recorder := serve(router, http.MethodPost, "/message-entries", body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "InvalidMessageEntryInputError" {
				t.Fatalf("code = %q", code)
			}
		})
	}
}

func TestCreateMessageEntryRejectsInvalidLocale(t *testing.T) {
	router := newTestRouter(t)
	locales := map[string]string{
		"empty":      ``,
		"whitespace": ` en`,
		"trailing":   `en `,
		"not a tag":  `not_a_tag`,
	}
	for name, locale := range locales {
		t.Run(name, func(t *testing.T) {
			body := `{"namespace":"a","locale":"` + locale + `","region":"US","prefix":"/a","content":"x"}`
			recorder := serve(router, http.MethodPost, "/message-entries", body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "InvalidLocaleInputError" {
				t.Fatalf("code = %q", code)
			}
		})
	}
}

func TestCreateMessageEntryValidationPrecedesConflict(t *testing.T) {
	router := newTestRouter(t)
	if recorder := serve(router, http.MethodPost, "/message-entries", validEntryBody); recorder.Code != http.StatusCreated {
		t.Fatalf("seed: status = %d", recorder.Code)
	}

	// Same namespace+locale but with an invalid region: validation wins over conflict.
	body := `{"namespace":"checkout","locale":"en-US","region":"USA","prefix":"/us","content":"Hello"}`
	recorder := serve(router, http.MethodPost, "/message-entries", body)
	if code := errorCode(t, recorder); code != "InvalidMessageEntryInputError" {
		t.Fatalf("code = %q, want InvalidMessageEntryInputError", code)
	}
}

func TestCreateMessageEntryReplayAndConflict(t *testing.T) {
	router := newTestRouter(t)
	first := serve(router, http.MethodPost, "/message-entries", validEntryBody)
	if first.Code != http.StatusCreated {
		t.Fatalf("seed: status = %d", first.Code)
	}

	replay := serve(router, http.MethodPost, "/message-entries", validEntryBody)
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay: status = %d, want %d", replay.Code, http.StatusCreated)
	}
	if replay.Body.String() != first.Body.String() {
		t.Fatalf("replay body = %s, want %s", replay.Body.String(), first.Body.String())
	}

	changed := `{"namespace":"checkout","locale":"en-US","region":"us","prefix":"/us","content":"Changed"}`
	recorder := serve(router, http.MethodPost, "/message-entries", changed)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("conflict: status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if code := errorCode(t, recorder); code != "LocaleConflictError" {
		t.Fatalf("code = %q", code)
	}

	// Region already bound to /us cannot be reused with another prefix.
	other := `{"namespace":"cart","locale":"fr","region":"US","prefix":"/elsewhere","content":"Bonjour"}`
	recorder = serve(router, http.MethodPost, "/message-entries", other)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("region conflict: status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if code := errorCode(t, recorder); code != "LocaleConflictError" {
		t.Fatalf("code = %q", code)
	}

	// Failed registrations leave nothing queryable behind.
	lookup := serve(router, http.MethodGet, "/route-mappings/fr?namespace=cart", "")
	if lookup.Code != http.StatusNotFound {
		t.Fatalf("lookup after conflict: status = %d, want %d", lookup.Code, http.StatusNotFound)
	}
}

func TestGetRouteMappingReturnsRegisteredFields(t *testing.T) {
	router := newTestRouter(t)
	if recorder := serve(router, http.MethodPost, "/message-entries", validEntryBody); recorder.Code != http.StatusCreated {
		t.Fatalf("seed: status = %d", recorder.Code)
	}

	recorder := serve(router, http.MethodGet, "/route-mappings/en-US?namespace=checkout", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if _, present := payload["content"]; present {
		t.Fatalf("route mapping must not expose content: %s", recorder.Body.String())
	}
	want := map[string]string{"namespace": "checkout", "locale": "en-US", "region": "US", "prefix": "/us"}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("%s = %q, want %q", key, payload[key], value)
		}
	}

	// Repeated queries and case variants of the locale return the same result.
	again := serve(router, http.MethodGet, "/route-mappings/EN-us?namespace=checkout", "")
	if again.Body.String() != recorder.Body.String() {
		t.Fatalf("repeat query body = %s, want %s", again.Body.String(), recorder.Body.String())
	}
}

func TestGetRouteMappingRejectsBadInput(t *testing.T) {
	router := newTestRouter(t)
	if recorder := serve(router, http.MethodPost, "/message-entries", validEntryBody); recorder.Code != http.StatusCreated {
		t.Fatalf("seed: status = %d", recorder.Code)
	}

	cases := []struct {
		name   string
		target string
		status int
		code   string
	}{
		{"invalid locale", "/route-mappings/not_a_tag?namespace=checkout", http.StatusBadRequest, "InvalidLocaleInputError"},
		{"missing namespace", "/route-mappings/en-US", http.StatusBadRequest, "InvalidQueryInputError"},
		{"blank namespace", "/route-mappings/en-US?namespace=%20%20", http.StatusBadRequest, "InvalidQueryInputError"},
		{"duplicate namespace", "/route-mappings/en-US?namespace=checkout&namespace=cart", http.StatusBadRequest, "InvalidQueryInputError"},
		{"unknown namespace", "/route-mappings/en-US?namespace=cart", http.StatusNotFound, "LocaleNotFoundError"},
		{"unknown locale", "/route-mappings/fr?namespace=checkout", http.StatusNotFound, "LocaleNotFoundError"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := serve(router, http.MethodGet, tc.target, "")
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, tc.status, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != tc.code {
				t.Fatalf("code = %q, want %q", code, tc.code)
			}
		})
	}
}

func TestRegistrationsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.db")

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if recorder := serve(NewRouter(st), http.MethodPost, "/message-entries", validEntryBody); recorder.Code != http.StatusCreated {
		t.Fatalf("seed: status = %d", recorder.Code)
	}
	st.Close()

	reopened, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	recorder := serve(NewRouter(reopened), http.MethodGet, "/route-mappings/en-US?namespace=checkout", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}
