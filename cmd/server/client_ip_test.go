package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// keyForRequest runs req through clientIPResolver(behindProxy) and reports the
// rate-limit bucket key credentialIPKey derives from it, exactly as the
// credential limiter would. An empty string means no trusted IP was resolved.
func keyForRequest(t *testing.T, behindProxy bool, req *http.Request) string {
	t.Helper()
	var key string
	r := chi.NewRouter()
	r.Use(clientIPResolver(behindProxy))
	r.Get("/", func(_ http.ResponseWriter, r *http.Request) {
		k, err := credentialIPKey(r)
		if err != nil {
			key = "" // errNoClientIP — the fail-closed path
			return
		}
		key = k
	})
	r.ServeHTTP(httptest.NewRecorder(), req)
	return key
}

// request builds a request from a fixed TCP peer, with optional X-Forwarded-For
// values supplied as separate header lines.
func request(remoteAddr string, xff ...string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for _, v := range xff {
		req.Header.Add("X-Forwarded-For", v)
	}
	return req
}

// TestClientIPDevTrustsOnlyTheSocket covers local dev, where nothing proxies
// us: the TCP peer is the client and forwarding headers are worthless.
func TestClientIPDevTrustsOnlyTheSocket(t *testing.T) {
	got := keyForRequest(t, false, request("203.0.113.7:54321", "198.51.100.9"))
	if got != "203.0.113.7" {
		t.Errorf("dev key = %q, want the socket peer %q (header must be ignored)", got, "203.0.113.7")
	}
}

// TestClientIPProxyTakesRightmostEntry pins the trust model: behind Railway we
// believe only the last X-Forwarded-For entry, the one appended by the hop
// closest to us.
func TestClientIPProxyTakesRightmostEntry(t *testing.T) {
	tests := []struct {
		name string
		xff  []string
		want string
	}{
		{"single entry set by the edge", []string{"203.0.113.7"}, "203.0.113.7"},
		{"client-supplied value then the edge's", []string{"198.51.100.9, 203.0.113.7"}, "203.0.113.7"},
		{"split across two header lines", []string{"198.51.100.9", "203.0.113.7"}, "203.0.113.7"},
		{"whitespace around entries", []string{" 198.51.100.9 ,  203.0.113.7 "}, "203.0.113.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyForRequest(t, true, request("10.0.0.1:443", tt.xff...)); got != tt.want {
				t.Errorf("key = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestClientIPProxyIgnoresSpoofedPrefix is the regression test for the
// vulnerability this replaced (GHSA-3fxj-6jh8-hvhx et al). chimiddleware.RealIP
// took the LEFTMOST entry, so an attacker rotating that value earned a fresh
// rate-limit bucket per request and the credential limiter did nothing. Every
// spoof below must collapse to the one bucket the edge actually vouched for.
func TestClientIPProxyIgnoresSpoofedPrefix(t *testing.T) {
	spoofs := []string{
		"1.1.1.1, 203.0.113.7",
		"2.2.2.2, 3.3.3.3, 203.0.113.7",
		"not-an-ip, 203.0.113.7",
		"::1, 203.0.113.7",
	}
	for _, xff := range spoofs {
		got := keyForRequest(t, true, request("10.0.0.1:443", xff))
		if got != "203.0.113.7" {
			t.Errorf("XFF %q gave bucket %q, want the edge-appended %q — the limit is evadable",
				xff, got, "203.0.113.7")
		}
	}
}

// TestClientIPProxyFailsClosed checks that a request with no usable
// X-Forwarded-For yields no key at all rather than silently falling back to the
// proxy's own address (which would pool every client into one bucket).
func TestClientIPProxyFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		xff  []string
	}{
		{"header absent entirely", nil},
		{"rightmost entry unparseable", []string{"203.0.113.7, garbage"}},
		{"header present but empty", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyForRequest(t, true, request("10.0.0.1:443", tt.xff...)); got != "" {
				t.Errorf("key = %q, want \"\" (errNoClientIP) — must not fall back to the proxy IP", got)
			}
		})
	}
}

// TestClientIPBucketsIPv6BySlash64 guards the CanonicalizeIP step. An IPv6
// client typically controls a whole /64, so keying the full address would let
// it walk its own range for an unlimited number of fresh buckets.
func TestClientIPBucketsIPv6BySlash64(t *testing.T) {
	first := keyForRequest(t, true, request("10.0.0.1:443", "2001:db8:1:2::1"))
	second := keyForRequest(t, true, request("10.0.0.1:443", "2001:db8:1:2:ffff:ffff:ffff:ffff"))
	if first != second {
		t.Errorf("addresses in one /64 gave different buckets %q and %q", first, second)
	}
	if first != "2001:db8:1:2::" {
		t.Errorf("bucket = %q, want the /64 prefix %q", first, "2001:db8:1:2::")
	}
}

// TestClientIPFoldsV4MappedIPv6 checks that a client cannot alias one address
// into two buckets by switching notation.
func TestClientIPFoldsV4MappedIPv6(t *testing.T) {
	plain := keyForRequest(t, true, request("10.0.0.1:443", "203.0.113.7"))
	mapped := keyForRequest(t, true, request("10.0.0.1:443", "::ffff:203.0.113.7"))
	if plain != mapped {
		t.Errorf("v4 %q and v4-mapped %q landed in different buckets", plain, mapped)
	}
}
