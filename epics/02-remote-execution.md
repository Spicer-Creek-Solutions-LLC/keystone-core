# Epic 2: Remote Execution System

## Overview

Implement a high-performance remote execution system that enables running commands, scripts, and executables across distributed infrastructure with real-time output streaming, targeting flexibility, and execution control.

**Goal**: Provide a Salt Project-like remote execution experience with modern performance, security, and usability improvements.

## Success Criteria

- [ ] Execute shell commands on targeted agents in <100ms
- [ ] Support flexible targeting (by ID, role, metadata, expressions)
- [ ] Real-time stdout/stderr streaming from all agents
- [ ] Parallel execution across 1000+ agents
- [ ] Support for file upload and execution
- [ ] Job history and result persistence
- [ ] Timeout, cancellation, and retry capabilities
- [ ] Execution success rate >99.9% (excluding agent offline)

## Architecture

```
┌────────────────────────────────────────────────────────┐
│                CLI / API Client                        │
│  titanctl exec "ls -la" --target "role:webserver"        │
└───────────────────┬────────────────────────────────────┘
                    │
                    ▼
┌────────────────────────────────────────────────────────┐
│              Execution Orchestrator                    │
│  ┌──────────────┐  ┌─────────────┐  ┌──────────────┐ │
│  │  Target      │  │  Job        │  │  Result      │ │
│  │  Resolver    │  │  Manager    │  │  Aggregator  │ │
│  └──────────────┘  └─────────────┘  └──────────────┘ │
└───────────────────┬────────────────────────────────────┘
                    │
                    ▼ (NATS pub/sub)
┌────────────────────────────────────────────────────────┐
│                  Agent Executors                       │
│  ┌─────────────────┐  ┌─────────────────┐            │
│  │  Command Exec   │  │  Script Exec    │            │
│  │  (shell, binary)│  │  (inline, file) │            │
│  └─────────────────┘  └─────────────────┘            │
│  ┌─────────────────┐  ┌─────────────────┐            │
│  │  Output Stream  │  │  Exit Handler   │            │
│  └─────────────────┘  └─────────────────┘            │
└────────────────────────────────────────────────────────┘
```

## User Stories

### US2.1: Simple Command Execution
**As an** operator
**I want to** execute shell commands on remote systems
**So that** I can perform operational tasks quickly

**Acceptance Criteria**:
- Execute arbitrary shell commands via CLI
- See real-time output from all targeted agents
- View exit codes and error status
- Support for common shells (bash, sh, powershell, cmd)
- Environment variable support
- Working directory specification

**Example**:
```bash
titanctl exec "systemctl status nginx" --target "role:webserver"
titanctl exec "df -h" --target "datacenter:us-east-1"
titanctl exec "Get-Service" --target "os:windows" --shell powershell
```

### US2.2: Flexible Targeting
**As an** operator
**I want to** target agents using flexible selection criteria
**So that** I can execute commands on the right set of systems

**Acceptance Criteria**:
- Target by agent ID: `--target "id:agent-123"`
- Target by role: `--target "role:database"`
- Target by metadata: `--target "env:production,region:us-west-2"`
- Target by glob pattern: `--target "web-*.example.com"`
- Target by compound expressions: `--target "role:web AND env:prod"`
- Target by percentage: `--target "role:web" --batch 10%`
- List matching agents before execution (dry-run)

**Example**:
```bash
titanctl exec "uptime" --target "role:web AND datacenter:us-*"
titanctl exec "reboot" --target "id:server-[1-10]" --batch 2
```

### US2.3: Script Execution
**As an** operator
**I want to** execute multi-line scripts on remote systems
**So that** I can perform complex operations

**Acceptance Criteria**:
- Execute inline scripts: `titanctl exec --script "#!/bin/bash\necho hello"`
- Execute script from file: `titanctl exec --script-file ./deploy.sh`
- Support multiple interpreters (bash, python, ruby, powershell)
- Pass arguments to scripts
- Upload dependencies with script
- Cleanup temporary files after execution

**Example**:
```bash
titanctl exec --script-file deploy.sh --args "v1.2.3" --target "role:app"
titanctl exec --script-file check.py --interpreter python3 --target "all"
```

### US2.4: File Transfer and Execution
**As an** operator
**I want to** upload binaries/files and execute them
**So that** I can deploy and run custom tools

**Acceptance Criteria**:
- Upload file to target agents
- Set executable permissions automatically
- Execute uploaded binary
- Support binary arguments
- Cleanup after execution (optional)
- Resume failed transfers

**Example**:
```bash
titanctl exec --upload ./collector --run --args "--config /etc/config.yml"
titanctl file upload ./config.yml --dest /etc/myapp/ --target "role:app"
```

### US2.5: Output Streaming and Aggregation
**As an** operator
**I want to** see real-time output from all agents
**So that** I can monitor execution progress

**Acceptance Criteria**:
- Real-time stdout/stderr streaming
- Color-coded output per agent
- Timestamp for each output line
- Output buffering for slow network
- Option to suppress output (quiet mode)
- Save output to file
- Aggregate similar outputs

**Example Output**:
```
[web-01] 2024-01-15 10:30:45 | STDOUT | nginx is running
[web-02] 2024-01-15 10:30:45 | STDOUT | nginx is running
[web-03] 2024-01-15 10:30:46 | STDERR | nginx is stopped
[web-03] 2024-01-15 10:30:46 | EXIT | code=1

Summary: 2 success, 1 failure
```

### US2.6: Job Management
**As an** operator
**I want to** manage long-running jobs
**So that** I can track and control execution

**Acceptance Criteria**:
- List running jobs: `titanctl job list`
- View job status: `titanctl job status <job-id>`
- Cancel running job: `titanctl job cancel <job-id>`
- View job results: `titanctl job result <job-id>`
- Job retention policy (configurable TTL)
- Job search and filtering

**Example**:
```bash
# Start long-running job
JOB_ID=$(titanctl exec "apt update && apt upgrade -y" --target "os:ubuntu" --async)

# Check status
titanctl job status $JOB_ID

# Cancel if needed
titanctl job cancel $JOB_ID
```

### US2.7: Execution Control
**As an** operator
**I want to** control execution behavior
**So that** I can handle different scenarios safely

**Acceptance Criteria**:
- Set timeout: `--timeout 30s`
- Retry on failure: `--retry 3`
- Batch execution: `--batch 10` (10 at a time)
- Rolling execution: `--batch 10% --wait-between 30s`
- Fail-fast mode: stop on first failure
- Best-effort mode: continue despite failures
- Dry-run mode: validate without executing

**Example**:
```bash
# Rolling restart with batches
titanctl exec "systemctl restart app" --target "role:app" --batch 20% --wait-between 60s

# Quick execution with timeout
titanctl exec "ping -c 1 google.com" --timeout 5s --target "all"
```

## Technical Tasks

### Phase 1: Core Execution Engine (Week 1-2)

**T1.1: Command Executor**
- Implement shell command execution (os/exec)
- Add support for different shells
- Implement environment variable handling
- Add working directory support
- Create process lifecycle management
- Add resource limits (CPU, memory, timeout)

**T1.2: Output Streaming**
- Implement real-time stdout/stderr capture
- Create line-buffered streaming over NATS
- Add output multiplexing (multiple agents)
- Implement backpressure handling
- Add output formatting and coloring

**T1.3: Exit Code and Error Handling**
- Capture process exit codes
- Differentiate between execution errors and agent errors
- Implement timeout handling
- Add signal handling (SIGTERM, SIGKILL)
- Create error reporting format

### Phase 2: Targeting System (Week 3)

**T2.1: Target Resolver**
- Parse target expressions
- Query agent registry by criteria
- Implement compound expressions (AND, OR, NOT)
- Add glob pattern matching
- Create target validation

**T2.2: Metadata Matching**
- Define standard metadata schema (role, env, datacenter, etc.)
- Implement metadata query engine
- Add custom metadata support
- Create metadata indexing for performance
- Support regex in metadata values

**T2.3: Batch Targeting**
- Implement percentage-based batching
- Add fixed-count batching
- Create rolling execution logic
- Add wait-between-batches support
- Implement failure handling per batch

### Phase 3: Script and File Support (Week 4)

**T3.1: Script Execution**
- Upload inline script to agent
- Execute script with specified interpreter
- Pass arguments to script
- Create temporary file management
- Implement cleanup after execution

**T3.2: File Transfer**
- Implement chunked file upload over NATS
- Add checksum verification
- Create progress tracking
- Implement resume capability
- Add compression for large files

**T3.3: Binary Execution**
- Upload binary to agent
- Set executable permissions
- Execute binary with arguments
- Handle different architectures (amd64, arm64)
- Implement signature verification (optional)

### Phase 4: Job Management (Week 5)

**T4.1: Job Tracking**
- Create job data model
- Implement job storage (state backend)
- Add job lifecycle management
- Create job UUID generation
- Implement job history retention

**T4.2: Job Control**
- Implement job cancellation
- Add job pause/resume (future)
- Create job status reporting
- Implement job result aggregation
- Add job cleanup scheduler

**T4.3: Async Execution**
- Support async job submission
- Implement job progress tracking
- Create job completion notifications
- Add webhook support for job events
- Implement job result retrieval

### Phase 5: CLI and API (Week 6)

**T5.1: CLI Commands**
- `titanctl exec` - Execute command
- `titanctl script` - Execute script
- `titanctl file` - File operations
- `titanctl job` - Job management
- Add shell completion support
- Create man pages

**T5.2: API Endpoints**
- `POST /api/v1/exec` - Execute command
- `GET /api/v1/jobs` - List jobs
- `GET /api/v1/jobs/{id}` - Get job status
- `DELETE /api/v1/jobs/{id}` - Cancel job
- `GET /api/v1/jobs/{id}/output` - Stream output (WebSocket)
- Add OpenAPI documentation

**T5.3: Result Formatting**
- JSON output format
- YAML output format
- Table output format
- Custom format templates
- Machine-readable vs human-readable modes

## Dependencies

- **Epic 1**: Core Infrastructure (NATS, agents, state)
- **Go Libraries**:
  - Standard library: `os/exec`, `io`, `bufio`
  - `github.com/expr-lang/expr` - Expression evaluation
  - `github.com/gobwas/glob` - Glob pattern matching
  - `github.com/google/uuid` - UUID generation
  - `github.com/fatih/color` - Terminal colors

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Command injection vulnerabilities | Critical | Medium | Strict input validation, use exec not shell |
| Output buffer overflow | Medium | Low | Implement backpressure, buffer limits |
| Network latency for large files | Medium | Medium | Chunking, compression, resume support |
| Process zombie accumulation | High | Low | Proper process reaping, timeout enforcement |
| Batch execution complexity | Medium | Medium | Comprehensive testing, clear error handling |

## Metrics & Monitoring

### Key Metrics
- Command execution latency (p50, p95, p99)
- Execution success rate (%)
- Output streaming lag (ms)
- File transfer throughput (MB/s)
- Concurrent jobs (gauge)
- Job completion time (histogram)

### Alerts
- Execution failure rate >1% over 5min
- Command timeout rate >5%
- File transfer failures
- Job queue depth >100
- Output streaming backpressure

## Testing Strategy

### Unit Tests
- Command execution with various shells
- Target expression parsing
- Output streaming with mock data
- File transfer chunking
- Job state transitions

### Integration Tests
- Multi-agent command execution
- Batch execution scenarios
- File upload and execution
- Job lifecycle (submit, track, cancel)
- Error scenarios (timeouts, agent offline)

### Performance Tests
- 1000 agents concurrent execution
- Large file transfers (1GB+)
- High-frequency job submission
- Output streaming throughput
- Target resolution performance

## Documentation Requirements

- [ ] Remote execution user guide
- [ ] Targeting syntax reference
- [ ] Script execution best practices
- [ ] Job management guide
- [ ] API reference with examples
- [ ] Security considerations
- [ ] Performance tuning guide
- [ ] Troubleshooting guide

## Definition of Done

- [ ] All user stories implemented and tested
- [ ] Unit test coverage >85%
- [ ] Integration tests passing
- [ ] Performance benchmarks met (<100ms latency)
- [ ] Security review completed
- [ ] Documentation complete
- [ ] CLI and API fully functional
- [ ] Demo scenarios validated
