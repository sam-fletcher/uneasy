---
title: "Secrets written by plans (Liaise, Spread Rumors) aren't announced, so open pages go stale"
order: 85
---

`clKeepSecretHandler` writes the meeting's secret straight to the database and
posts the "entrusted the meeting's secret to ..." log line, but unlike the plain
write-secret route (`assets_secrets.go`) it broadcasts no `secret.created`. Open
clients therefore keep the asset's old `secret_count`. The visible symptom is in
Things We Share: "Look at partner's secrets" lists only assets with a secret the
viewer cannot yet read, so the partner's freshly kept secret is missing and the
option says "Your partner has no assets with secrets you can't already read"
until the page is reloaded. The same stale count shows on every other open
retinue card.

Spread Rumors has the same hole twice: the secret stash
(`plan_spread_rumors_routes.go`, the preparer's secret rumor) and hide-source
both create a secret with `CreateSecret` and announce nothing; only
`assets_secrets.go` broadcasts `secret.created`.

Fix: one create-and-announce helper (create the secret, broadcast
`secret.created` with asset id and author only) used by all four sites.

Pinned as a `test.fixme` in `frontend/tests/e2e/clandestinely_liaise.spec.ts`
("the partner can look at a freshly kept secret without reloading"); un-fix it
when this lands, and drop the `alice.reload()` workaround in the hidden-secret
test.
