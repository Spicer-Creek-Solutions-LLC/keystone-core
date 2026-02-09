---
title: Resource Management
description: Module resource limits and usage tracking
weight: 20
---

Package resources provides configurable resource limits for Keystone plugin modules.

**Import:** `github.com/shawnbutts/keystone-core/pkg/plugin/resources`

## Contents

- [Enforcer](#enforcer)
- [LimitEvent](#limitevent)
- [LimitEventListener](#limiteventlistener)
- [LimitedContext](#limitedcontext)
- [Limits](#limits)
- [Pool](#pool)
- [PoolStats](#poolstats)
- [ResourceManager](#resourcemanager)
- [Usage](#usage)
- [usage](#usage)

## Variables

### ErrCPUTimeExceeded

Common errors.

**Type:** ``

### ErrMemoryExceeded

Common errors.

**Type:** ``

### ErrTimeoutExceeded

Common errors.

**Type:** ``

### ErrNetworkDisabled

Common errors.

**Type:** ``

### ErrFilesystemDisabled

Common errors.

**Type:** ``

### ErrIOPSExceeded

Common errors.

**Type:** ``

## Enforcer

Enforcer enforces resource limits.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `limits` | `*Limits` |  |
| `startTime` | `time.Time` |  |
| `usage` | `*usage` |  |
| `stopCh` | `chan *ast.StructType` |  |
| `doneCh` | `chan *ast.StructType` |  |
| `mu` | `sync.RWMutex` |  |
| `listeners` | `[]LimitEventListener` |  |
| `violated` | `error` |  |

### Methods

#### AddListener

```go
func (e *Enforcer) AddListener(listener LimitEventListener)
```

AddListener adds an event listener.

**Parameters:**

- `listener` (`LimitEventListener`)

#### CheckExec

```go
func (e *Enforcer) CheckExec() error
```

CheckExec checks if subprocess execution is allowed.

**Returns:**

- `error`

#### CheckFileSize

```go
func (e *Enforcer) CheckFileSize(size int64) error
```

CheckFileSize checks if a file size is within limits.

**Parameters:**

- `size` (`int64`)

**Returns:**

- `error`

#### CheckFilesystem

```go
func (e *Enforcer) CheckFilesystem() error
```

CheckFilesystem checks if filesystem access is allowed.

**Returns:**

- `error`

#### CheckNetwork

```go
func (e *Enforcer) CheckNetwork() error
```

CheckNetwork checks if network access is allowed.

**Returns:**

- `error`

#### RecordBytes

```go
func (e *Enforcer) RecordBytes(read, written int64)
```

RecordBytes records bytes read or written.

**Parameters:**

- `read` (`int64`)
- `written` (`int64`)

#### RecordGoroutine

```go
func (e *Enforcer) RecordGoroutine() error
```

RecordGoroutine records a goroutine being started.

**Returns:**

- `error`

#### RecordIOPS

```go
func (e *Enforcer) RecordIOPS() error
```

RecordIOPS records an I/O operation.

**Returns:**

- `error`

#### RecordMemory

```go
func (e *Enforcer) RecordMemory(bytes int64) error
```

RecordMemory records memory usage.

**Parameters:**

- `bytes` (`int64`)

**Returns:**

- `error`

#### RecordNetworkConn

```go
func (e *Enforcer) RecordNetworkConn() error
```

RecordNetworkConn records a network connection.

**Returns:**

- `error`

#### RecordOpenFile

```go
func (e *Enforcer) RecordOpenFile() error
```

RecordOpenFile records a file being opened.

**Returns:**

- `error`

#### ReleaseGoroutine

```go
func (e *Enforcer) ReleaseGoroutine()
```

ReleaseGoroutine releases a goroutine.

#### ReleaseMemory

```go
func (e *Enforcer) ReleaseMemory(bytes int64)
```

ReleaseMemory releases memory.

**Parameters:**

- `bytes` (`int64`)

#### ReleaseNetworkConn

```go
func (e *Enforcer) ReleaseNetworkConn()
```

ReleaseNetworkConn releases a network connection.

#### ReleaseOpenFile

```go
func (e *Enforcer) ReleaseOpenFile()
```

ReleaseOpenFile releases an open file.

#### Start

```go
func (e *Enforcer) Start(ctx context.Context) error
```

Start starts enforcing limits.

**Parameters:**

- `ctx` (`context.Context`)

**Returns:**

- `error`

#### Stop

```go
func (e *Enforcer) Stop()
```

Stop stops enforcing limits.

#### Usage

```go
func (e *Enforcer) Usage() Usage
```

Usage returns current usage.

**Returns:**

- `Usage`

#### Violated

```go
func (e *Enforcer) Violated() error
```

Violated returns the first violated limit error, if any.

**Returns:**

- `error`

#### emit

```go
func (e *Enforcer) emit(event *LimitEvent)
```

**Parameters:**

- `event` (`*LimitEvent`)

#### monitor

```go
func (e *Enforcer) monitor(ctx context.Context)
```

**Parameters:**

- `ctx` (`context.Context`)

#### setViolated

```go
func (e *Enforcer) setViolated(err error)
```

**Parameters:**

- `err` (`error`)

---

## LimitEvent

LimitEvent represents a limit event.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `Type` | `string` |  |
| `Limit` | `string` |  |
| `Current` | `int64` |  |
| `Maximum` | `int64` |  |
| `Timestamp` | `time.Time` |  |

---

## LimitEventListener

LimitEventListener is called when limit events occur.

---

## LimitedContext

LimitedContext wraps a context with resource limit checking.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `enforcer` | `*Enforcer` |  |

### Methods

#### Enforcer

```go
func (lc *LimitedContext) Enforcer() *Enforcer
```

Enforcer returns the enforcer.

**Returns:**

- `*Enforcer`

#### Err

```go
func (lc *LimitedContext) Err() error
```

Err returns context error or limit violation.

**Returns:**

- `error`

---

## Limits

Limits defines resource limits for a module.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `CPUTime` | `time.Duration` | CPUTime is the maximum CPU time allowed. |
| `WallTime` | `time.Duration` | WallTime is the maximum wall clock time allowed. |
| `Memory` | `int64` | Memory is the maximum memory in bytes. |
| `MaxGoroutines` | `int` | MaxGoroutines is the maximum number of goroutines. |
| `MaxFileSize` | `int64` | MaxFileSize is the maximum file size in bytes. |
| `MaxOpenFiles` | `int` | MaxOpenFiles is the maximum number of open files. |
| `MaxIOPS` | `int64` | MaxIOPS is the maximum I/O operations per second. |
| `MaxNetworkConns` | `int` | MaxNetworkConns is the maximum network connections. |
| `AllowNetwork` | `bool` | AllowNetwork enables network access. |
| `AllowFilesystem` | `bool` | AllowFilesystem enables filesystem access. |
| `AllowExec` | `bool` | AllowExec enables subprocess execution. |

---

## Pool

Pool manages resource pools.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` |  |
| `total` | `int64` |  |
| `available` | `int64` | atomic |
| `mu` | `sync.Mutex` |  |

### Methods

#### Acquire

```go
func (p *Pool) Acquire(ctx context.Context, amount int64) error
```

Acquire acquires resources from the pool.

**Parameters:**

- `ctx` (`context.Context`)
- `amount` (`int64`)

**Returns:**

- `error`

#### Available

```go
func (p *Pool) Available() int64
```

Available returns available resources.

**Returns:**

- `int64`

#### Release

```go
func (p *Pool) Release(amount int64)
```

Release releases resources back to the pool.

**Parameters:**

- `amount` (`int64`)

#### Total

```go
func (p *Pool) Total() int64
```

Total returns total resources.

**Returns:**

- `int64`

#### TryAcquire

```go
func (p *Pool) TryAcquire(amount int64) bool
```

TryAcquire tries to acquire without blocking.

**Parameters:**

- `amount` (`int64`)

**Returns:**

- `bool`

---

## PoolStats

PoolStats contains pool statistics.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` |  |
| `Total` | `int64` |  |
| `Available` | `int64` |  |
| `Used` | `int64` |  |

---

## ResourceManager

ResourceManager manages multiple resource pools.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `pools` | `map[string]*Pool` |  |
| `mu` | `sync.RWMutex` |  |

### Methods

#### CreatePool

```go
func (rm *ResourceManager) CreatePool(name string, total int64) *Pool
```

CreatePool creates a new pool.

**Parameters:**

- `name` (`string`)
- `total` (`int64`)

**Returns:**

- `*Pool`

#### GetPool

```go
func (rm *ResourceManager) GetPool(name string) *Pool
```

GetPool returns a pool by name.

**Parameters:**

- `name` (`string`)

**Returns:**

- `*Pool`

#### Stats

```go
func (rm *ResourceManager) Stats() map[string]PoolStats
```

Stats returns stats for all pools.

**Returns:**

- `map[string]PoolStats`

---

## Usage

Usage tracks resource usage.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `CPUTime` | `time.Duration` |  |
| `WallTime` | `time.Duration` |  |
| `Memory` | `int64` |  |
| `PeakMemory` | `int64` |  |
| `Goroutines` | `int` |  |
| `OpenFiles` | `int64` |  |
| `IOPS` | `int64` |  |
| `NetworkConns` | `int64` |  |
| `BytesRead` | `int64` |  |
| `BytesWritten` | `int64` |  |

---

## usage

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `cpuTime` | `int64` | nanoseconds, atomic |
| `memory` | `int64` | atomic |
| `peakMemory` | `int64` | atomic |
| `goroutines` | `int64` | atomic |
| `openFiles` | `int64` | atomic |
| `iops` | `int64` | atomic |
| `networkConns` | `int64` | atomic |
| `bytesRead` | `int64` | atomic |
| `bytesWritten` | `int64` | atomic |

---

---

*Generated by moddoc*
