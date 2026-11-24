// Package ops holds the server's self-monitoring: the two alerts that go to the
// owner's Discord (docs/OPERATIONS.md "Alerts").
//
//   - DBMonitor + DBTracer: notice that database requests have started failing
//     and post at most once an hour while they keep failing.
//   - BackupWatch: once a day, check that the weekly backup workflow has
//     reported in recently.
//
// Everything here is deliberately passive. Nothing in this package queries the
// database to find out whether it is up (Neon bills awake time, and a health
// probe would keep it awake all month); the monitor listens to the requests the
// app is already making, and the backup check rides on the notify ticker's
// existing wake-up. The clock and the Discord poster are injected so tests need
// neither real time nor a real webhook.
package ops
