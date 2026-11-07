---
title: "Tell players when the live connection drops"
order: 71
---

A dropped WebSocket is only logged to the console while the client retries with
backoff up to 30 seconds. A tab can sit open for hours showing a board that stopped
updating. Deferred from the error-handling work as its own feature. It's the one case
where a global notice is the right shape.
