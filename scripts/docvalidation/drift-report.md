# Documentation Drift Report

## Summary

| Category | Count |
|----------|-------|
| Packages without docs | 24 |
| Undocumented types | 0 |
| Undocumented functions | 177 |
| Stale references | 11 |
| Epics without docs | 0 |

## Packages Without Documentation

- `audit`
- `backup`
- `cloud`
- `controlplane`
- `credentials`
- `edge`
- `execution`
- `files`
- `hardware`
- `identity`
- `k8s`
- `metrics`
- `nats`
- `netutil`
- `platform`
- `plugin`
- `profiling`
- `proxy`
- `selfmgmt`
- `servicemesh`
- `targeting`
- `tracing`
- `vendors`
- `visualization`

## Undocumented Functions

### audit (2 functions)

- `Log` (backends.go:18)
- `Close` (backends.go:22)

### backup (4 functions)

- `Debug` (manager.go:457)
- `Info` (manager.go:458)
- `Warn` (manager.go:459)
- `Error` (manager.go:460)

### bootstrap (6 functions)

- `Debug` (bootstrap.go:472)
- `Info` (bootstrap.go:473)
- `Warn` (bootstrap.go:474)
- `Error` (bootstrap.go:475)
- `Error` (config.go:181)
- `Error` (config.go:188)

### cloud (2 functions)

- `String` (types.go:19)
- `String` (types.go:48)

### container (1 functions)

- `String` (types.go:21)

### controlplane (2 functions)

- `ListAgents` (batch_dispatcher.go:55)
- `GetAgent` (batch_dispatcher.go:68)

### edge (1 functions)

- `String` (types.go:17)

### events (57 functions)

- `Name` (actions.go:38)
- `Type` (actions.go:42)
- `Execute` (actions.go:46)
- `Name` (actions.go:98)
- `Type` (actions.go:102)
- `Execute` (actions.go:106)
- `Name` (actions.go:165)
- `Type` (actions.go:169)
- `Execute` (actions.go:173)
- `Name` (actions.go:239)
- `Type` (actions.go:243)
- `Execute` (actions.go:247)
- `Name` (actions.go:287)
- `Type` (actions.go:291)
- `Execute` (actions.go:295)
- `Name` (actions.go:317)
- `Type` (actions.go:321)
- `Execute` (actions.go:325)
- `Name` (actions.go:352)
- `Type` (actions.go:356)
- `Execute` (actions.go:360)
- `Name` (actions.go:390)
- `Type` (actions.go:394)
- `Execute` (actions.go:398)
- `Name` (actions.go:444)
- `Type` (actions.go:448)
- `Execute` (actions.go:452)
- `Name` (actions.go:491)
- `Type` (actions.go:495)
- `Execute` (actions.go:499)
- `Publish` (cloudevents.go:215)
- `PublishAsync` (cloudevents.go:226)
- `Close` (cloudevents.go:236)
- `Close` (cloudevents.go:280)
- `Matches` (filter_expression.go:48)
- `String` (filter_expression.go:75)
- `Matches` (filter_expression.go:246)
- `String` (filter_expression.go:259)
- `Name` (enrichment.go:120)
- `Enrich` (enrichment.go:124)
- `Name` (enrichment.go:152)
- `Enrich` (enrichment.go:156)
- `Name` (enrichment.go:177)
- `Enrich` (enrichment.go:181)
- `Name` (enrichment.go:202)
- `Enrich` (enrichment.go:206)
- `Name` (enrichment.go:226)
- `Enrich` (enrichment.go:230)
- `Name` (enrichment.go:252)
- `Enrich` (enrichment.go:256)
- `Name` (enrichment.go:285)
- `Enrich` (enrichment.go:289)
- `Name` (enrichment.go:316)
- `Enrich` (enrichment.go:320)
- `Publish` (enrichment.go:363)
- `PublishAsync` (enrichment.go:373)
- `Close` (enrichment.go:383)

### execution (20 functions)

- `Name` (shell.go:90)
- `Type` (shell.go:94)
- `Command` (shell.go:98)
- `IsAvailable` (shell.go:102)
- `EnvVarSeparator` (shell.go:107)
- `Name` (shell.go:114)
- `Type` (shell.go:118)
- `Command` (shell.go:122)
- `IsAvailable` (shell.go:126)
- `EnvVarSeparator` (shell.go:131)
- `Name` (shell.go:138)
- `Type` (shell.go:142)
- `Command` (shell.go:146)
- `IsAvailable` (shell.go:152)
- `EnvVarSeparator` (shell.go:162)
- `Name` (shell.go:169)
- `Type` (shell.go:173)
- `Command` (shell.go:177)
- `IsAvailable` (shell.go:182)
- `EnvVarSeparator` (shell.go:191)

### logging (8 functions)

- `Int` (types.go:135)
- `Int64` (types.go:139)
- `Float64` (types.go:143)
- `Bool` (types.go:147)
- `Duration` (types.go:151)
- `Time` (types.go:155)
- `Error` (types.go:159)
- `Any` (types.go:166)

### nats (59 functions)

- `Allow` (degradation.go:260)
- `SetRate` (degradation.go:279)
- `String` (health.go:25)
- `String` (health.go:417)
- `Name` (health.go:607)
- `Check` (health.go:611)
- `Name` (health.go:647)
- `Check` (health.go:651)
- `String` (websocket.go:253)
- `String` (websocket.go:859)
- `Dial` (websocket.go:1053)
- `Name` (websocket.go:1407)
- `SupportsEndpoint` (websocket.go:1411)
- `ConfigureOptions` (websocket.go:1415)
- `Priority` (websocket.go:1442)
- `String` (discovery.go:48)
- `Method` (discovery.go:226)
- `Discover` (discovery.go:230)
- `Watch` (discovery.go:369)
- `Close` (discovery.go:399)
- `Method` (discovery.go:491)
- `Discover` (discovery.go:495)
- `Watch` (discovery.go:504)
- `Close` (discovery.go:534)
- `Method` (discovery.go:666)
- `Discover` (discovery.go:670)
- `Watch` (discovery.go:903)
- `Close` (discovery.go:933)
- `Method` (discovery.go:1041)
- `Discover` (discovery.go:1052)
- `Watch` (discovery.go:1313)
- `Close` (discovery.go:1343)
- `Method` (discovery.go:1401)
- `Discover` (discovery.go:1405)
- `Watch` (discovery.go:1409)
- `Close` (discovery.go:1414)
- `String` (connection_manager.go:30)
- `Reset` (ntlm.go:494)
- `Write` (ntlm.go:503)
- `Sum` (ntlm.go:525)
- `Len` (endpoint.go:430)
- `Less` (endpoint.go:431)
- `Swap` (endpoint.go:432)
- `Name` (strategy.go:111)
- `SupportsEndpoint` (strategy.go:115)
- `ConfigureOptions` (strategy.go:119)
- `Priority` (strategy.go:185)
- `Name` (strategy.go:202)
- `SupportsEndpoint` (strategy.go:206)
- `ConfigureOptions` (strategy.go:210)
- `Priority` (strategy.go:296)
- `Name` (strategy.go:313)
- `SupportsEndpoint` (strategy.go:317)
- `ConfigureOptions` (strategy.go:321)
- `Priority` (strategy.go:355)
- `Name` (strategy.go:372)
- `SupportsEndpoint` (strategy.go:376)
- `ConfigureOptions` (strategy.go:382)
- `Priority` (strategy.go:411)

### selfmgmt (6 functions)

- `Debug` (types.go:68)
- `Info` (types.go:69)
- `Warn` (types.go:70)
- `Error` (types.go:71)
- `Error` (types.go:796)
- `Error` (types.go:803)

### servicemesh (1 functions)

- `String` (types.go:23)

### statemgmt (1 functions)

- `Error` (types.go:262)

### tracing (2 functions)

- `ExportSpans` (provider.go:282)
- `Shutdown` (provider.go:286)

### upgrade (4 functions)

- `Debug` (version.go:591)
- `Info` (version.go:592)
- `Warn` (version.go:593)
- `Error` (version.go:594)

### vendors (1 functions)

- `Error` (types.go:400)

## Stale References

| Doc File | Reference | Type |
|----------|-----------|------|
| concepts/events.md | events.EventTypeJobComplete | symbol |
| concepts/events.md | events.SeverityInfo | symbol |
| concepts/events.md | events.NewQuery | symbol |
| concepts/events.md | events.EventTypeAgentConnect | symbol |
| concepts/events.md | events.SeverityWarning | symbol |
| concepts/events.md | events.WithFilter | symbol |
| concepts/events.md | events.WithTransform | symbol |
| concepts/events.md | events.NewReplay | symbol |
| reference/api.md | pkg/client | package |
| reference/events.md | events.EventTypeJobComplete | symbol |
| reference/events.md | events.SeverityInfo | symbol |

