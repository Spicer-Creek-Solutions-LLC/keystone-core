// Package outbound is the Keystone Core outbound-webhooks subsystem
// (Epic 16 tasks 11–18, PROJECT-DETAILS §4.14): persistent webhook
// subscriptions + delivery audit + a NATS-driven Manager that fans
// events out to a [Dispatcher] (HTTP POST + HMAC-signed) with a
// retry queue and per-endpoint circuit breaker.
//
// Task 11 scope (this file's package): the persistence layer only —
// the [Subscription] / [DeliveryRecord] value types and the
// [SubscriptionStore] interface, with parallel [MemoryStore] and
// [SQLiteStore] implementations. The remaining pieces land in their
// own tasks:
//
//   - Manager (event-bus subscriber, glob filter, async fan-out)  — task 12
//   - Dispatcher (HTTP POST + custom headers + HMAC + timeout)    — task 13
//   - RetryQueue (exp backoff + jitter)                           — task 14
//   - Per-endpoint circuit breaker                                — task 15
//   - REST + `kscore-webhook outbound` CLI                        — task 16
//   - Sign/Verify helpers (`sha256=<hex>`)                        — task 17
//   - End-to-end integration test                                 — task 18
package outbound
