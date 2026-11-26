package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Pages under /dev/ are team tools, not content for search. The header is set
// by the static handler, not by a dev-only route, so it holds whether or not
// the dev routes are mounted, and a crawler that runs no JavaScript still
// sees it on the response.
func TestDevPathsAreNoindex(t *testing.T) {
	r := prodFrontend(t)

	resp := get(t, r, "/dev/styleguide", false)
	assert.Equal(t, "noindex", resp.Header.Get("X-Robots-Tag"), "/dev/ pages must ask crawlers to skip them")

	resp = get(t, r, "/", false)
	assert.Empty(t, resp.Header.Get("X-Robots-Tag"), "the public front page must not be marked noindex")
}
