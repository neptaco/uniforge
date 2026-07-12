package hub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestResolveChangeset(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		q := r.URL.Query()
		if q.Get("version") != "2022.3.99f9" {
			t.Errorf("version = %q, want 2022.3.99f9", q.Get("version"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"total": 1,
			"results": [
				{
					"version": "2022.3.99f9",
					"shortRevision": "abcdef012345",
					"stream": "LTS",
					"releaseDate": "2024-01-15T00:00:00.000Z",
					"recommended": false,
					"releaseNotes": {"url": "https://example.com/notes.md", "type": "MD"},
					"downloads": []
				}
			]
		}`)
	}))
	defer server.Close()

	withTestReleaseAPI(t, server.URL)

	changeset, err := ResolveChangeset("2022.3.99f9")
	if err != nil {
		t.Fatalf("ResolveChangeset failed: %v", err)
	}
	if changeset != "abcdef012345" {
		t.Errorf("changeset = %q, want %q", changeset, "abcdef012345")
	}

	// Second call must be served from cache (no additional HTTP request)
	changeset2, err := ResolveChangeset("2022.3.99f9")
	if err != nil {
		t.Fatalf("ResolveChangeset (cached) failed: %v", err)
	}
	if changeset2 != "abcdef012345" {
		t.Errorf("cached changeset = %q, want %q", changeset2, "abcdef012345")
	}
	if calls.Load() != 1 {
		t.Errorf("Expected 1 HTTP request, got %d", calls.Load())
	}
}

func TestResolveChangeset_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"total": 0, "results": []}`)
	}))
	defer server.Close()

	withTestReleaseAPI(t, server.URL)

	_, err := ResolveChangeset("2099.9.99f9")
	if err == nil {
		t.Fatal("Expected error for unknown version, got nil")
	}
}

func TestResolveChangeset_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"status":404,"title":"Not Found","code":54}`)
	}))
	defer server.Close()

	withTestReleaseAPI(t, server.URL)

	_, err := ResolveChangeset("2098.8.88f8")
	if err == nil {
		t.Fatal("Expected error for HTTP 404, got nil")
	}
}
