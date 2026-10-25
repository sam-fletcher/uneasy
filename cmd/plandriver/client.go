package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// client is one player's session: a cookie jar plus the dev-login that fills
// it. Each player needs their own, because the session cookie is per-origin —
// logging in as someone else replaces it.
type client struct {
	base     string
	username string
	http     *http.Client
}

func newClient(base, username string) (*client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &client{
		base:     base,
		username: username,
		http:     &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}, nil
}

// call sends one request and decodes the JSON body into out (which may be nil
// when the response is ignored). A non-2xx is an error carrying the server's
// own message — those messages are the useful part when a prepare is refused.
func (c *client) call(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	// G704 (SSRF) does not apply: the host comes from the operator's own
	// -base flag on a dev-only CLI, so there is no untrusted input to taint it.
	//nolint:gosec // the base URL is a developer-supplied flag, not request data
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	//nolint:gosec // same as above: the request URL originates from the -base flag
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach %s — is the dev stack up? (docker compose up): %w", c.base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s → %d as %s: %s",
			method, path, resp.StatusCode, c.username, bytes.TrimSpace(raw))
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *client) get(path string, out any) error { return c.call(http.MethodGet, path, nil, out) }
func (c *client) post(path string, body, out any) error {
	if body == nil {
		body = map[string]any{}
	}
	return c.call(http.MethodPost, path, body, out)
}

// login opens a session via the dev shortcut, filling this client's jar.
func (c *client) login() error {
	return c.post("/api/dev/login?username="+c.username, nil, nil)
}

// ── Response shapes (only the fields the driver reads) ───────────────────────

type player struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
}

type asset struct {
	ID              int64  `json:"id"`
	OwnerID         int64  `json:"owner_id"`
	AssetType       string `json:"asset_type"`
	Name            string `json:"name"`
	IsMainCharacter bool   `json:"is_main_character"`
	IsDestroyed     bool   `json:"is_destroyed"`
}

type plan struct {
	ID        int64  `json:"id"`
	PlanType  string `json:"plan_type"`
	RowNumber *int16 `json:"row_number"`
	Status    string `json:"status"`
}

type diceRoll struct {
	ID      int64   `json:"id"`
	Result  *int16  `json:"result"`
	Outcome *string `json:"outcome"`
}

type seedResponse struct {
	GameID  int64    `json:"game_id"`
	Players []player `json:"players"`
}
