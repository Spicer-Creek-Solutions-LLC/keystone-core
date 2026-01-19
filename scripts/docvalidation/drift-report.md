# Documentation Drift Report

## Summary

| Category | Count |
|----------|-------|
| Packages without docs | 22 |
| Undocumented types | 0 |
| Undocumented functions | 179 |
| Stale references | 10 |
| Epics without docs | 0 |

## Packages Without Documentation

- `audit`
- `cloud`
- `controlplane`
- `credentials`
- `edge`
- `execution`
- `files`
- `hardware`
- `identity`
- `logging`
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

## Undocumented Functions

### audit (2 functions)

- `Log` (backends.go:18)
- `Close` (backends.go:22)

### backup (4 functions)

- `Debug` (manager.go:463)
- `Info` (manager.go:464)
- `Warn` (manager.go:465)
- `Error` (manager.go:466)

### blueprint (1 functions)

- `Error` (validator.go:21)

### bootstrap (6 functions)

- `Debug` (bootstrap.go:486)
- `Info` (bootstrap.go:487)
- `Warn` (bootstrap.go:488)
- `Error` (bootstrap.go:489)
- `Error` (config.go:181)
- `Error` (config.go:188)

### cloud (2 functions)

- `String` (types.go:19)
- `String` (types.go:48)

### container (1 functions)

- `String` (types.go:72)

### controlplane (2 functions)

- `ListAgents` (batch_dispatcher.go:55)
- `GetAgent` (batch_dispatcher.go:68)

### edge (1 functions)

- `String` (types.go:17)

### events (57 functions)

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
- `Publish` (cloudevents.go:215)
- `PublishAsync` (cloudevents.go:226)
- `Close` (cloudevents.go:236)
- `Close` (cloudevents.go:280)
- `Matches` (filter_expression.go:48)
- `String` (filter_expression.go:75)
- `Matches` (filter_expression.go:246)
- `String` (filter_expression.go:259)
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

- `String` (discovery.go:49)
- `Method` (discovery.go:227)
- `Discover` (discovery.go:231)
- `Watch` (discovery.go:370)
- `Close` (discovery.go:400)
- `Method` (discovery.go:492)
- `Discover` (discovery.go:496)
- `Watch` (discovery.go:597)
- `Close` (discovery.go:627)
- `Method` (discovery.go:759)
- `Discover` (discovery.go:763)
- `Watch` (discovery.go:996)
- `Close` (discovery.go:1026)
- `Method` (discovery.go:1134)
- `Discover` (discovery.go:1145)
- `Watch` (discovery.go:1406)
- `Close` (discovery.go:1436)
- `Method` (discovery.go:1494)
- `Discover` (discovery.go:1498)
- `Watch` (discovery.go:1502)
- `Close` (discovery.go:1507)
- `Reset` (ntlm.go:494)
- `Write` (ntlm.go:503)
- `Sum` (ntlm.go:525)
- `String` (connection_manager.go:30)
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
- `Allow` (degradation.go:260)
- `SetRate` (degradation.go:279)
- `String` (websocket.go:253)
- `String` (websocket.go:860)
- `Dial` (websocket.go:1054)
- `Name` (websocket.go:1408)
- `SupportsEndpoint` (websocket.go:1412)
- `ConfigureOptions` (websocket.go:1416)
- `Priority` (websocket.go:1443)
- `String` (health.go:25)
- `String` (health.go:417)
- `Name` (health.go:607)
- `Check` (health.go:611)
- `Name` (health.go:647)
- `Check` (health.go:651)

### security (1 functions)

- `Error` (path.go:15)

### selfmgmt (6 functions)

- `Debug` (types.go:68)
- `Info` (types.go:69)
- `Warn` (types.go:70)
- `Error` (types.go:71)
- `Error` (types.go:796)
- `Error` (types.go:803)

### servicemesh (1 functions)

- `String` (types.go:33)

### statemgmt (1 functions)

- `Error` (types.go:291)

### tracing (2 functions)

- `ExportSpans` (provider.go:282)
- `Shutdown` (provider.go:286)

### upgrade (4 functions)

- `Debug` (version.go:601)
- `Info` (version.go:602)
- `Warn` (version.go:603)
- `Error` (version.go:604)

### vendors (1 functions)

- `Error` (types.go:400)

## Stale References

| Doc File | Reference | Type |
|----------|-----------|------|
| concepts/events.md | events.EventTypeJobComplete | symbol |
| concepts/events.md | events.SeverityInfo | symbol |
| concepts/events.md | events.EventTypeAgentConnect | symbol |
| concepts/events.md | events.SeverityWarning | symbol |
| concepts/events.md | events.EventTypeStateApplyStart | symbol |
| concepts/events.md | events.EventTypeStateApplyDone | symbol |
| concepts/events.md | events.EventTypeStateChange | symbol |
| concepts/events.md | events.EventTypeStateDrift | symbol |
| reference/events.md | events.EventTypeJobComplete | symbol |
| reference/events.md | events.SeverityInfo | symbol |

