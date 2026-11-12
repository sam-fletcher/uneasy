---
title: "Should the server keep crawlers off /dev/styleguide, not only the page?"
---

`/dev/styleguide` ships to the live site and carries `<meta name="robots" content="noindex, nofollow">`.
But the app is a client-rendered single page: the server answers every unknown path with `index.html`,
and the tag is added by the page's code once it runs. A crawler that doesn't run JavaScript never sees
it, and one that does must run the page first.

Nothing links to the route, so a crawler would have to guess the address. The page holds no game data
and no rule text. If you want a stronger guarantee than the meta tag, the options are:

- **An `X-Robots-Tag: noindex` header** from the Go server on any path under `/dev/`. A few lines in the
  static-file handler. This works for every crawler, with or without JavaScript.
- **A `robots.txt`** with `Disallow: /dev/`. The app has none today, so this would add the first one
  (which also lets you say something about the rest of the site).
- **Nothing more.** The meta tag is what the brief asked for, and the exposure is a page of tokens and
  placeholder names.

This is a Go change in a piece that is otherwise all frontend, so I left it.
