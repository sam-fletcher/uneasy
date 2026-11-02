package handler

import (
	"context"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// secrets.go — the one door every new Secret goes through.
//
// Owner ruling (2026-11-01): how many secrets an asset hides, and which asset
// holds them, is public; only the secret's text is hidden. So any secret, from
// any route, must (1) be stored, (2) broadcast secret.created so open pages
// bump the asset's count, and (3) post one log line naming the asset — never
// the text. Wiring those by hand per route is how routes came to skip the
// broadcast or invent their own wording.
//
// TestSecretsAreCreatedThroughTheHelper fails if a handler calls CreateSecret
// anywhere but here.

// secretNotice says where a new secret's log line is anchored and whose name
// it carries.
type secretNotice struct {
	Row    *int16 // logRow(game), or the plan's row
	PlanID *int64 // anchors the line to a plan when one caused the secret
	// HideAuthor keeps the author's name out of the line (Spread Rumors'
	// hide-source: the fiction hides who spread the rumor). The asset is still
	// named.
	HideAuthor bool
}

// createSecret stores a secret and nothing else. It exists for a caller inside
// a transaction (Spread Rumors' prepare stash), which must announce only after
// the commit; call announceSecret then. Everyone else uses writeSecret.
func createSecret(
	ctx context.Context, q *dbgen.Queries, assetID, authorID int64, text string,
) (dbgen.Secret, error) {
	return q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID:  assetID,
		AuthorID: authorID,
		Text:     text,
	})
}

// announceSecret broadcasts secret.created (asset and author ids, no text) and
// posts the log line.
func announceSecret(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	asset dbgen.Asset,
	authorID int64,
	n secretNotice,
) {
	broadcastEvent(manager, asset.GameID, model.EventSecretCreated, model.SecretCreatedPayload{
		AssetID:  asset.ID,
		AuthorID: authorID,
	})
	EmitSecretWritten(ctx, q, manager, asset, authorID, n)
}

// writeSecret stores a secret and announces it. Best-effort announcing: the
// secret is already stored by then.
func writeSecret(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	asset dbgen.Asset,
	authorID int64,
	text string,
	n secretNotice,
) (dbgen.Secret, error) {
	secret, err := createSecret(ctx, q, asset.ID, authorID, text)
	if err != nil {
		return secret, err
	}
	announceSecret(ctx, q, manager, asset, authorID, n)
	return secret, nil
}
