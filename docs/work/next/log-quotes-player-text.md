---
title: "Log lines quote player text with Go's %q: straight quotes, escaped marks, a literal \\n"
order: 77
---

Several action-log lines quote what a player wrote using `fmt.Sprintf("… %q", text)`. `%q` gives a
Go string literal. The text gets straight double quotes, a `"` the player typed shows as `\"`, and a
line break shows as a literal `\n`. The house style for a player's own words is curly marks, with
the text as typed (`Quote`, "Inline text" in `docs/STYLE_GUIDE.md`). Found while reviewing #146,
whose treaty closing line follows the same pattern.

**Fix:** add one helper in `handler` that wraps text in curly marks (“…”) and changes nothing inside
them. Use it for every log line that quotes player text:

- `asset_effects.go` (~71): the torn marginalia;
- `laws_rumors.go` (~143, ~241): a law or rumor edited;
- `plan_host_festivity_options.go` (~124, ~127, ~163): a rumor spread at the event, and a new peer's
  marginalia;
- `plan_make_introductions.go` (~949): a newcomer's marginalia;
- `plan_propose_decree.go` (~392, ~401): the enacted decree and its resource's marginalia;
- `plan_make_war_peace.go`: the peace closing line that #146 adds (once #146 has merged).

Leave the rest: `plan_chronicle_histories.go` (~809) quotes an option's name, not player text, and
the `%q`s in error responses are for developers.

Check that the chat shows a line break inside a log line as a break. The decree's line already
carries `\n\n`, so it probably does. If it doesn't, say so in the PR rather than changing the feed.

**Done when:** a unit test of the helper covers a typed `"` and a line break; the tests that read
these lines expect curly marks; a screenshot of one log line quoting two-line text in the chat at
375px.
