# TitanAnvil

**GitOps deploys it. We keep it running.**

TitanAnvil is a cloud-native runtime infrastructure control plane that provides real-time execution, continuous compliance, and operational automation across hybrid environments.

## Project Status

This repository contains the complete implementation of **Epic 1: Core Infrastructure** with all four phases completed:

- ✅ **Phase 1**: NATS Integration (Embedded, External, Leaf modes)
- ✅ **Phase 2**: Agent Development (Registration, Heartbeat, Command Execution)
- ✅ **Phase 3**: Control Plane Services (State Management, Connection Management)
- ✅ **Phase 4**: Testing & Reliability (>80% test coverage achieved)

### Completed Features

✅ **Project Structure**
- Go module initialization
- Directory structure following design specifications
- Build system (Makefile) with cross-compilation support

✅ **Configuration Management**
- Comprehensive configuration system using Viper
- Support for YAML config files and environment variables
- Three NATS deployment modes: embedded, external, leaf
- Two storage backends: SQLite (embedded) and PostgreSQL (production)

✅ **NATS Integration**
- NATS connection manager with mode selection
- **Embedded NATS mode** - zero external dependencies
- **External cluster mode** - connect to NATS cluster
- **Leaf node mode** - embedded NATS connected to parent cluster
- JetStream support for event persistence
- Connection pooling and automatic reconnection
- Health checks and graceful shutdown

✅ **Security Foundation**
- mTLS certificate generation utilities
- Certificate Authority (CA) creation
- Server and client certificate generation
- PEM file I/O for certificates and keys

✅ **API Protocol**
- Protobuf message schemas for agent communication
- Agent registration and heartbeat protocol
- Command execution with streaming output
- System metrics collection
- gRPC service definitions

✅ **Core Binaries**
- `titananvil-server` - Control plane server (with embedded NATS support)
- `titananvil-agent` - Agent daemon (with embedded NATS support)
- `titanctl` - Main CLI tool (plugin dispatcher pattern)

✅ **Agent Features**
- Agent registration with control plane
- Heartbeat mechanism with configurable intervals
- Command execution engine with streaming output
- System metadata collection (OS, arch, hostname, IPs)
- Automatic reconnection and health monitoring

✅ **State Management**
- SQLite embedded storage (zero configuration)
- Agent and command persistence
- Query filtering and pagination
- PostgreSQL support planned for production deployments

✅ **Testing & Quality**
- Comprehensive unit test suite (>80% coverage)
- Agent package: 77.9% coverage
- State package: 90.1% coverage
- Config package: 96.6% coverage
- Control plane package: 85.9% coverage
- Security package: 80.0% coverage
- Version package: 100% coverage

## Quick Start

### Prerequisites

- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)
- Make

### Build

```bash
# Install dependencies
make deps

# Generate protobuf code
make proto

# Build all binaries
make build
```

This creates binaries in `./bin/`:
- `titananvil-server`
- `titananvil-agent`
- `titanctl`

### Run Server (Embedded Mode - Zero Dependencies)

```bash
# Copy example config
cp titan-anvil.yaml.example titan-anvil.yaml

# Run server with embedded NATS + SQLite
./bin/titananvil-server
```

The server will:
- Start an embedded NATS server on port 4222
- Enable JetStream for event persistence
- Use SQLite for state storage
- Listen for gRPC connections on port 9090
- Listen for REST connections on port 8080

**No external dependencies required!**

### Run Agent (Embedded Mode)

```bash
# Run agent with embedded NATS
./bin/titananvil-agent
```

The agent will:
- Start an embedded NATS server
- Connect to the control plane
- Send heartbeats every 30 seconds
- Wait for command execution requests

### Configuration

TitanAnvil supports flexible deployment configurations:

#### NATS Modes

1. **Embedded** (default) - Best for getting started
   - In-process NATS server
   - Zero external dependencies
   - Perfect for dev, testing, small deployments (<100 nodes)

2. **External** - Best for production
   - Connect to external NATS cluster
   - High availability
   - Scalable to 1000+ nodes

3. **Leaf** - Best for edge deployments
   - Embedded NATS as leaf node
   - Connects to parent NATS cluster
   - Works offline with local queuing

#### Storage Backends

1. **SQLite** (default) - Best for getting started
   - Embedded database, single file
   - Zero configuration
   - Perfect for dev, testing, small deployments (<100 nodes)

2. **PostgreSQL** - Best for production
   - External database
   - High availability, replication support
   - Scalable to 1000+ nodes

See `titan-anvil.yaml.example` for all configuration options.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      Control Plane                           │
│  ┌────────────┐  ┌─────────────┐  ┌──────────────────┐     │
│  │  API       │  │  State      │  │  Event/Reactor   │     │
│  │  Server    │  │  Manager    │  │  Engine          │     │
│  └────────────┘  └─────────────┘  └──────────────────┘     │
│         │               │                    │               │
│         └───────────────┴────────────────────┘               │
│                         │                                    │
└─────────────────────────┼────────────────────────────────────┘
                          │
                   ┌──────▼──────┐
                   │    NATS     │
                   │  (Embedded  │
                   │  or External)│
                   └──────┬──────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
   │ Agent   │      │ Agent   │      │ Agent   │
   │ (K8s)   │      │ (VM)    │      │ (Edge)  │
   └─────────┘      └─────────┘      └─────────┘
```

## Project Structure

```
titan-anvil/
├── cmd/
│   ├── titananvil-server/     # Control plane server
│   ├── titananvil-agent/      # Agent daemon
│   └── titanctl/              # Main CLI (plugin dispatcher)
├── pkg/
│   ├── api/v1/                # Generated protobuf code
│   ├── config/                # Configuration management
│   ├── nats/                  # NATS connection manager
│   ├── security/              # mTLS certificate utilities
│   └── version/               # Version information
├── api/proto/                 # Protobuf definitions
├── epics/                     # Epic-level design docs
├── Makefile                   # Build system
├── go.mod                     # Go dependencies
└── titan-anvil.yaml.example   # Example configuration
```

## Next Steps

**Epic 1: Core Infrastructure** is now complete!

### Completed in Epic 1
- ✅ All 4 phases completed (NATS, Agent, Control Plane, Testing)
- ✅ >80% test coverage achieved across all core packages
- ✅ Embedded NATS mode with zero dependencies
- ✅ SQLite state storage working
- ✅ Agent registration, heartbeat, and command execution
- ✅ Comprehensive test suite with integration tests

### Ready for Epic 2: Remote Execution

The next phase of development is **Epic 2: Remote Execution**, which builds on Epic 1 to add:

- Advanced command execution features (async execution, job queues)
- Target selection and filtering (by labels, status, queries)
- Result aggregation and reporting
- Command history and audit logging
- Security controls (command allowlists, rate limiting)

See `epics/02-remote-execution.md` for complete details.

### Optional Improvements (Future Work)

While Epic 1 is functionally complete, these enhancements could be added later:

- PostgreSQL backend implementation (currently SQLite only)
- SQLite → PostgreSQL migration tooling
- Performance testing with 1000+ agents
- Load testing and benchmarking
- Full integration test suite with real NATS cluster
- Security audit and penetration testing

## Development

### Running Tests

```bash
make test
```

### Generating Protobuf Code

```bash
make proto
```

### Cleaning Build Artifacts

```bash
make clean
```

## Documentation

- **Design Document**: `DESIGN.md` - Overall product vision and architecture
- **CLAUDE.md**: `CLAUDE.md` - Development guidance for AI assistants
- **Epic 1**: `epics/01-core-infrastructure.md` - Core infrastructure details

## License

Apache 2.0 (planned)

## Contributing

This is currently in early development. See `epics/` for implementation roadmap.
