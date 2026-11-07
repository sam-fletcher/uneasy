---
title: "Two failures nothing would tell you about"
order: 76
---

- `/healthz` must stay DB-free, so the uptime pinger can't see a database outage. If
  Neon suspends past its cap, nothing alerts.
- GitHub disables cron workflows after about 60 days without repo activity, and
  nothing alerts on a missed weekly backup.
