package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// prodFrontend builds the router the production branch of setupFrontend
// installs, serving the real embedded build.
func prodFrontend(t *testing.T) *chi.Mux {
	t.Helper()
	r := chi.NewRouter()
	if err := setupFrontend(r, false, ""); err != nil {
		t.Fatalf("setupFrontend: %v", err)
	}
	return r
}

// anImmutableAsset returns the path of some file under _app/immutable/,
// discovered from the embed rather than hardcoded — every build rewrites
// these hashes.
func anImmutableAsset(t *testing.T) string {
	t.Helper()
	sub, err := fs.Sub(frontendFS, "frontend_dist")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	var found string
	err = fs.WalkDir(sub, "_app/immutable", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && path.Ext(p) == ".js" && found == "" {
			found = p
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("no immutable .js asset in the embedded build (walk err: %v)", err)
	}
	return found
}

func get(t *testing.T, r http.Handler, url string, acceptGzip bool) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Result()
}

func TestStaticAssetsAreCompressed(t *testing.T) {
	r := prodFrontend(t)
	asset := anImmutableAsset(t)

	resp := get(t, r, "/"+asset, true)
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip — JS is shipping uncompressed", got)
	}

	// A client that doesn't ask for gzip must still get a working response.
	resp = get(t, r, "/"+asset, false)
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q for a client that didn't accept gzip, want none", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestCacheControlByPath(t *testing.T) {
	r := prodFrontend(t)

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"hashed asset is pinned", "/" + anImmutableAsset(t), cacheImmutable},
		{"font is cached for a month", "/fonts/spectral-regular.woff2", cacheFonts},
		{"index revalidates", "/", cacheRevalidate},
		{"service worker revalidates", "/service-worker.js", cacheRevalidate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, r, tt.url, true)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}

// The SPA fallback serves index.html with a 200 for any path that isn't in
// the build — including a stale _app/immutable/ URL from a tab that was open
// across a redeploy. Marking *that* response immutable would pin a dead index
// in the browser for a year, turning a reload-to-fix into a permanent break.
func TestSPAFallbackIsNotCachedAsImmutable(t *testing.T) {
	r := prodFrontend(t)

	resp := get(t, r, "/_app/immutable/nodes/999.NoSuchHash.js", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback)", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html — expected the SPA fallback here", ct)
	}
	if got := resp.Header.Get("Cache-Control"); got != cacheRevalidate {
		t.Errorf("Cache-Control = %q, want %q", got, cacheRevalidate)
	}

	// Same for a plain client-side route.
	resp = get(t, r, "/table/42", true)
	if got := resp.Header.Get("Cache-Control"); got != cacheRevalidate {
		t.Errorf("client-side route: Cache-Control = %q, want %q", got, cacheRevalidate)
	}
}
