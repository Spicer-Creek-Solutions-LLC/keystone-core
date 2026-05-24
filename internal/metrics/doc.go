// SPDX-License-Identifier: Apache-2.0

// Package metrics is the Keystone Core Prometheus abstraction layer.
//
// The package owns a private *prometheus.Registry (NOT the default global)
// and exposes thin Counter/Gauge/Histogram/Summary interfaces over it,
// plus a Timer utility and a cardinality limiter. Subpackages emit by
// calling the Registry constructors at init time; the /metrics HTTP
// handler (wired in Epic 17 task 3) scrapes the Registry's Gatherer.
//
// Design notes:
//
//   - One process-wide Registry per server. Tests construct their own.
//   - Collectors are returned via constructor methods on Registry rather
//     than free functions so the Registry can attach its limiter and so
//     re-registration of the same metric name is rejected at the source.
//   - The cardinality limiter runs inline on every labeled observation.
//     It tracks unique label-value combinations per metric and either
//     drops or aggregates new combinations once the per-metric cap is
//     reached. The limiter's accept/drop/aggregate counts are themselves
//     exposed as kscore_metrics_cardinality_total{metric,outcome}.
//   - Buckets/Objectives default to prometheus.DefBuckets / DefObjectives
//     when MetricDef leaves them empty. Histograms always observe in
//     seconds; the Timer utility records time.Since(start).Seconds() so
//     dashboards can rely on the unit.
//
// See PROJECT-DETAILS.md §4.16 and epics/17-observability.md.
package metrics
