package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kvmymluq2i/cnv-locale-router/internal/store"
)

func newTestRouter(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st, NewRouter(st)
}

func doRequest(handler http.Handler, method, target string, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, reader)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func validBody() string {
	return `{"namespace":"billing","locale":"en-us","region":"us","prefix":"/us/checkout","content":"Pay now"}`
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
		t.Fatalf("response is not JSON: %s", recorder.Body.String())
	}
	if payload.Error.Code == "" || payload.Error.Message == "" {
		t.Fatalf("error object missing code/message: %s", recorder.Body.String())
	}
	if len(mustJSONMap(t, recorder.Body.String())) != 1 {
		t.Fatalf("error response must have a single top-level key: %s", recorder.Body.String())
	}
	return payload.Error.Code
}

func mustJSONMap(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatalf("not json: %s", body)
	}
	return top
}

func TestPostMessageEntrySuccess(t *testing.T) {
	_, handler := newTestRouter(t)
	recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody())

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{
		"namespace": "billing",
		"locale":    "en-US",
		"region":    "US",
		"prefix":    "/us/checkout",
		"content":   "Pay now",
	}
	if len(got) != len(want) {
		t.Fatalf("response has %d fields, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestPostIdenticalRepeatSucceeds(t *testing.T) {
	_, handler := newTestRouter(t)
	first := doRequest(handler, http.MethodPost, "/message-entries", validBody())
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d: %s", first.Code, first.Body.String())
	}
	second := doRequest(handler, http.MethodPost, "/message-entries", validBody())
	if second.Code != http.StatusCreated {
		t.Fatalf("repeat status = %d, want 201: %s", second.Code, second.Body.String())
	}
	if second.Body.String() != first.Body.String() {
		t.Fatalf("repeat body = %s, want %s", second.Body.String(), first.Body.String())
	}
}

func TestPostInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"array", `[1,2,3]`},
		{"string scalar", `"hello"`},
		{"number", `42`},
		{"two objects", validBody() + validBody()},
		{"empty object", `{}`},
		{"missing namespace", `{"locale":"en","region":"US","prefix":"/p","content":"c"}`},
		{"missing locale", `{"namespace":"n","region":"US","prefix":"/p","content":"c"}`},
		{"missing region", `{"namespace":"n","locale":"en","prefix":"/p","content":"c"}`},
		{"missing prefix", `{"namespace":"n","locale":"en","region":"US","content":"c"}`},
		{"missing content", `{"namespace":"n","locale":"en","region":"US","prefix":"/p"}`},
		{"wrong type namespace", `{"namespace":1,"locale":"en","region":"US","prefix":"/p","content":"c"}`},
		{"null locale", `{"namespace":"n","locale":null,"region":"US","prefix":"/p","content":"c"}`},
		{"region as number", `{"namespace":"n","locale":"en","region":1,"prefix":"/p","content":"c"}`},
		{"content array", `{"namespace":"n","locale":"en","region":"US","prefix":"/p","content":["c"]}`},
		{"blank namespace", `{"namespace":"   ","locale":"en","region":"US","prefix":"/p","content":"c"}`},
		{"empty content", `{"namespace":"n","locale":"en","region":"US","prefix":"/p","content":""}`},
		{"region too short", `{"namespace":"n","locale":"en","region":"U","prefix":"/p","content":"c"}`},
		{"region three letters", `{"namespace":"n","locale":"en","region":"USA","prefix":"/p","content":"c"}`},
		{"region digits", `{"namespace":"n","locale":"en","region":"U1","prefix":"/p","content":"c"}`},
		{"region non-ascii", `{"namespace":"n","locale":"en","region":"美区","prefix":"/p","content":"c"}`},
		{"prefix no leading slash", `{"namespace":"n","locale":"en","region":"US","prefix":"p","content":"c"}`},
		{"prefix empty", `{"namespace":"n","locale":"en","region":"US","prefix":"","content":"c"}`},
		{"prefix with question mark", `{"namespace":"n","locale":"en","region":"US","prefix":"/p?x","content":"c"}`},
		{"prefix with hash", `{"namespace":"n","locale":"en","region":"US","prefix":"/p#x","content":"c"}`},
		{"prefix with space", `{"namespace":"n","locale":"en","region":"US","prefix":"/p x","content":"c"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, handler := newTestRouter(t)
			recorder := doRequest(handler, http.MethodPost, "/message-entries", tc.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "InvalidMessageEntryInputError" {
				t.Fatalf("code = %s, want InvalidMessageEntryInputError", code)
			}
		})
	}
}

func TestPostInvalidLocaleAfterOtherValidation(t *testing.T) {
	// Invalid locale but otherwise valid -> InvalidLocaleInputError.
	body := `{"namespace":"n","locale":"english","region":"US","prefix":"/p","content":"c"}`
	_, handler := newTestRouter(t)
	recorder := doRequest(handler, http.MethodPost, "/message-entries", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "InvalidLocaleInputError" {
		t.Fatalf("code = %s, want InvalidLocaleInputError", code)
	}

	// Invalid locale AND invalid region -> the non-locale error wins.
	both := `{"namespace":"n","locale":"english","region":"USA","prefix":"/p","content":"c"}`
	recorder = doRequest(handler, http.MethodPost, "/message-entries", both)
	if code := errorCode(t, recorder); code != "InvalidMessageEntryInputError" {
		t.Fatalf("code = %s, want InvalidMessageEntryInputError (input validation precedes locale)", code)
	}

	// Invalid locale on an otherwise-valid request that WOULD conflict still
	// reports the locale error: input validation precedes conflict detection.
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
		t.Fatalf("seed: %s", recorder.Body.String())
	}
	conflictButBadLocale := `{"namespace":"billing","locale":"not_a_tag","region":"FR","prefix":"/fr","content":"Autre"}`
	recorder = doRequest(handler, http.MethodPost, "/message-entries", conflictButBadLocale)
	if code := errorCode(t, recorder); code != "InvalidLocaleInputError" {
		t.Fatalf("code = %s, want InvalidLocaleInputError before conflict", code)
	}
}

func TestPostConflicts(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"different content", `{"namespace":"billing","locale":"en-US","region":"US","prefix":"/us/checkout","content":"Pay later"}`},
		{"different region", `{"namespace":"billing","locale":"en-US","region":"CA","prefix":"/us/checkout","content":"Pay now"}`},
		{"different prefix same region", `{"namespace":"billing","locale":"en-US","region":"US","prefix":"/us/other","content":"Pay now"}`},
		{"region reused with other prefix", `{"namespace":"billing","locale":"fr-FR","region":"us","prefix":"/us/other","content":"Payer"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, handler := newTestRouter(t)
			if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
				t.Fatalf("seed: %s", recorder.Body.String())
			}
			recorder := doRequest(handler, http.MethodPost, "/message-entries", tc.body)
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "LocaleConflictError" {
				t.Fatalf("code = %s, want LocaleConflictError", code)
			}

			// The original registration must survive unchanged.
			after := doRequest(handler, http.MethodPost, "/message-entries", validBody())
			if after.Code != http.StatusCreated {
				t.Fatalf("original entry no longer repeatable: %d", after.Code)
			}
		})
	}
}

func TestPostSharedRegionPrefixAcrossLocales(t *testing.T) {
	_, handler := newTestRouter(t)
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
		t.Fatalf("seed: %s", recorder.Body.String())
	}
	other := `{"namespace":"billing","locale":"fr-FR","region":"us","prefix":"/us/checkout","content":"Payer"}`
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", other); recorder.Code != 201 {
		t.Fatalf("shared region/prefix status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetRouteMappingSuccess(t *testing.T) {
	_, handler := newTestRouter(t)
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
		t.Fatalf("seed: %s", recorder.Body.String())
	}

	recorder := doRequest(handler, http.MethodGet, "/route-mappings/en-us?namespace=billing", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{"namespace": "billing", "locale": "en-US", "region": "US", "prefix": "/us/checkout"}
	if len(got) != 4 {
		t.Fatalf("response must expose exactly four fields without content: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestGetRouteMappingQueryValidation(t *testing.T) {
	_, handler := newTestRouter(t)
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
		t.Fatalf("seed: %s", recorder.Body.String())
	}

	cases := []struct {
		name   string
		target string
	}{
		{"missing namespace", "/route-mappings/en-US"},
		{"blank namespace", "/route-mappings/en-US?namespace=%20%20"},
		{"duplicate namespace", "/route-mappings/en-US?namespace=billing&namespace=billing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doRequest(handler, http.MethodGet, tc.target, "")
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "InvalidQueryInputError" {
				t.Fatalf("code = %s, want InvalidQueryInputError", code)
			}
		})
	}
}

func TestGetRouteMappingInvalidLocale(t *testing.T) {
	_, handler := newTestRouter(t)
	// Bad locale beats missing namespace validation.
	recorder := doRequest(handler, http.MethodGet, "/route-mappings/english", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "InvalidLocaleInputError" {
		t.Fatalf("code = %s, want InvalidLocaleInputError", code)
	}
}

func TestGetRouteMappingNotFound(t *testing.T) {
	_, handler := newTestRouter(t)
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
		t.Fatalf("seed: %s", recorder.Body.String())
	}

	cases := []string{
		"/route-mappings/fr-FR?namespace=billing", // other locale, no fallback
		"/route-mappings/en?namespace=billing",    // broader tag, no fallback
		"/route-mappings/en-US?namespace=other",   // other namespace
	}
	for _, target := range cases {
		recorder := doRequest(handler, http.MethodGet, target, "")
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404: %s", target, recorder.Code, recorder.Body.String())
		}
		if code := errorCode(t, recorder); code != "LocaleNotFoundError" {
			t.Fatalf("code = %s, want LocaleNotFoundError", code)
		}
	}
}

func TestRepeatedAndReorderedReadsAreStable(t *testing.T) {
	st, handler := newTestRouter(t)
	seed := func(locale, region, prefix, content string) {
		body := `{"namespace":"billing","locale":"` + locale + `","region":"` + region + `","prefix":"` + prefix + `","content":"` + content + `"}`
		if recorder := doRequest(handler, http.MethodPost, "/message-entries", body); recorder.Code != 201 {
			t.Fatalf("seed %s: %s", locale, recorder.Body.String())
		}
	}
	seed("en-US", "US", "/us/checkout", "Pay")
	seed("fr-FR", "FR", "/fr", "Payer")
	seed("ja-JP", "JP", "/jp", "支払う")

	targets := []string{
		"/route-mappings/ja-jp?namespace=billing",
		"/route-mappings/en-us?namespace=billing",
		"/route-mappings/fr-fr?namespace=billing",
		"/route-mappings/en-us?namespace=billing",
	}
	want := map[string]string{
		"ja-JP": "/jp",
		"en-US": "/us/checkout",
		"fr-FR": "/fr",
	}
	first := map[string]string{}
	for i, target := range targets {
		recorder := doRequest(handler, http.MethodGet, target, "")
		if recorder.Code != 200 {
			t.Fatalf("read %d: %d %s", i, recorder.Code, recorder.Body.String())
		}
		var got struct {
			Locale string `json:"locale"`
			Prefix string `json:"prefix"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &got)
		if got.Prefix != want[got.Locale] {
			t.Fatalf("read %d: locale %s prefix %s", i, got.Locale, got.Prefix)
		}
		if i == 0 {
			first[got.Locale] = recorder.Body.String()
		}
	}
	_ = st
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.db")

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	handler := NewRouter(st)
	if recorder := doRequest(handler, http.MethodPost, "/message-entries", validBody()); recorder.Code != 201 {
		t.Fatalf("seed: %s", recorder.Body.String())
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	recorder := doRequest(NewRouter(st2), http.MethodGet, "/route-mappings/en-US?namespace=billing", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status after restart = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestExistingSurfaceUnchanged(t *testing.T) {
	_, handler := newTestRouter(t)

	healthz := doRequest(handler, http.MethodGet, "/healthz", "")
	if healthz.Code != http.StatusOK || healthz.Body.String() != `{"database":"ok","status":"ok"}` {
		t.Fatalf("healthz changed: %d %s", healthz.Code, healthz.Body.String())
	}
	missing := doRequest(handler, http.MethodGet, "/nope", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d", missing.Code)
	}
	if code := errorCode(t, missing); code != "route_not_found" {
		t.Fatalf("unknown path code = %s", code)
	}
}
