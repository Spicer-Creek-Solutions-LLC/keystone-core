# Glossary

This glossary defines security, infrastructure, and domain-specific terminology used throughout the Keystone Core documentation and codebase.

## Security Terms

### Attestation

The process by which an agent proves its identity and integrity to the control plane. Attestation methods include join tokens, cloud instance metadata, Kubernetes service account tokens, and TPM-based attestation.

### Authentication

The process of verifying the identity of a user, agent, or service. Keystone Core supports multiple authentication methods including API keys, JWT tokens, mTLS certificates, and SPIFFE SVIDs.

### Authorization

The process of determining whether an authenticated entity has permission to perform a specific action. Implemented via OPA (Rego) and CEL policy engines.

### CA (Certificate Authority)

An entity that issues digital certificates. Keystone Core includes an embedded CA for issuing agent certificates and can integrate with external CAs.

### CEL (Common Expression Language)

A non-Turing-complete expression language used for policy evaluation. Provides a simpler alternative to OPA/Rego for basic authorization rules.

### Cipher Suite

A combination of cryptographic algorithms used to secure network connections. Includes algorithms for key exchange, authentication, encryption, and message authentication.

### Credential

Secret information used for authentication, such as passwords, API keys, certificates, or tokens.

### Defense in Depth

A security strategy that layers multiple independent security controls so that failure of one control does not compromise the entire system.

### Encryption at Rest

Protection of data stored on disk through cryptographic encryption. Used for database contents, backups, and sensitive configuration files.

### Encryption in Transit

Protection of data as it moves across networks through cryptographic protocols like TLS.

### Fail Secure (Fail Closed)

A design principle where system failures result in a secure state (typically denying access) rather than granting access.

### HMAC (Hash-based Message Authentication Code)

A cryptographic construction for verifying both data integrity and authenticity using a secret key and a hash function.

### IAM (Identity and Access Management)

The framework of policies and technologies for ensuring that the right entities have appropriate access to resources.

### Join Token

A one-time or limited-use secret used by agents to register with the control plane. Provides initial attestation before certificate issuance.

### Key Derivation Function (KDF)

A cryptographic function that derives one or more secret keys from a secret value such as a password. Examples include Argon2, scrypt, and PBKDF2.

### Least Privilege

The security principle that entities should have only the minimum permissions necessary to perform their functions.

### mTLS (Mutual TLS)

TLS authentication where both the client and server present certificates, providing mutual identity verification.

### OPA (Open Policy Agent)

A policy engine that evaluates policies written in Rego. Used for fine-grained authorization decisions.

### RBAC (Role-Based Access Control)

An access control model where permissions are assigned to roles, and roles are assigned to users.

### Rego

The policy language used by Open Policy Agent (OPA) for defining authorization rules.

### Secret

Any sensitive data that should be protected, including passwords, API keys, certificates, encryption keys, and tokens.

### SPIFFE (Secure Production Identity Framework For Everyone)

A set of standards for identifying and securing communications between services. Provides a universal identity format (SPIFFE ID) and certificate format (SVID).

### SPIFFE ID

A URI that uniquely identifies a workload within a trust domain. Format: `spiffe://trust-domain/path`

### SVID (SPIFFE Verifiable Identity Document)

A cryptographic document (X.509 certificate or JWT) that proves a workload's SPIFFE identity.

### TLS (Transport Layer Security)

A cryptographic protocol that provides privacy and data integrity between applications communicating over a network.

### Token

A security artifact that represents identity or authorization claims. Can be short-lived (JWT) or long-lived (API key).

### Trust Boundary

A point in a system where the level of trust changes, requiring security controls such as authentication, authorization, and input validation.

### Trust Domain

The administrative boundary within which SPIFFE identities are issued and validated. Corresponds to a PKI trust root.

### X.509

The standard format for public key certificates used in TLS and SPIFFE.

### Zero Trust

A security model that requires verification for every access request, regardless of network location or previous authentication.

## Infrastructure Terms

### Agent

A lightweight process that runs on managed nodes, communicating with the control plane to execute commands and apply state configurations.

### Bootstrap

The process of initializing a new Keystone Core deployment or registering a new agent, including certificate provisioning and initial configuration.

### Cluster

A group of control plane servers working together for high availability, using etcd for consensus and distributed coordination.

### Control Plane

The central management layer that coordinates agents, stores state, evaluates policies, and processes commands.

### Drift

The difference between the declared (desired) state and the actual state of a managed system.

### Edge Agent

An agent deployed in environments with intermittent connectivity, capable of operating autonomously with local state caching.

### Embedded NATS

NATS server running in-process with the control plane for simplified deployment without external dependencies.

### etcd

A distributed key-value store used for cluster coordination, leader election, and distributed locking.

### Heartbeat

A periodic message sent by agents to the control plane to indicate health and connectivity.

### High Availability (HA)

A system design that minimizes downtime through redundancy, automatic failover, and distributed operation.

### JetStream

NATS's built-in persistence layer providing at-least-once and exactly-once message delivery with stream storage.

### Leaf Node

A NATS connection mode where agents connect to a local NATS server that relays messages to the main cluster.

### NATS

A lightweight, high-performance messaging system used for all control plane to agent communication.

### Proxy Agent

An agent that manages devices unable to run native agents, using protocols like SSH, SNMP, or WinRM.

### Quorum

The minimum number of cluster members that must agree for distributed operations to proceed (typically majority).

### State

The declared configuration for a managed system, expressed as a collection of state modules with their parameters.

### State Module

A unit of configuration that manages a specific aspect of a system (e.g., file, package, service, user).

### Supercluster

A NATS deployment spanning multiple geographic regions connected via gateways for global agent management.

### Targeting

The process of selecting which agents should receive a command or state application, using glob patterns or filter expressions.

## Protocol Terms

### gRPC

A high-performance RPC framework used for the client API (CLI tools communicating with control plane).

### OTLP (OpenTelemetry Protocol)

The standard protocol for transmitting telemetry data (metrics, logs, traces) in cloud-native environments.

### REST

Representational State Transfer, an architectural style for web APIs. Used for webhook receivers and external integrations.

### SNMP (Simple Network Management Protocol)

A protocol for managing and monitoring network devices. Keystone Core proxy agents support SNMPv2c and SNMPv3.

### SSH (Secure Shell)

A protocol for secure remote access and command execution. Used by proxy agents for managing Unix/Linux systems.

### WebSocket

A protocol for full-duplex communication over a single TCP connection. Used for real-time agent communication through firewalls.

### WinRM (Windows Remote Management)

A Microsoft protocol for remote management of Windows systems. Used by proxy agents for Windows device management.

## Development Terms

### Capability

A permission granted to a module that controls what operations it can perform (file system, network, execution, etc.).

### CEL Expression

A filter expression using Common Expression Language syntax for targeting agents or filtering events.

### Module

An extension that provides additional state management or execution capabilities, running in a sandboxed environment.

### Policy

A set of rules that govern authorization decisions, written in Rego (OPA) or CEL.

### Reactor

An event-driven automation that executes actions in response to specific events based on filter conditions.

### Requisite

A dependency relationship between state modules (require, watch, prereq, onchanges) that controls execution order.

### Starlark

A Python-like configuration language used for writing modules. Executes in a secure sandbox.

### WASM (WebAssembly)

A portable binary format for executable code. Used for running modules in a secure, sandboxed environment.

## Abbreviations

| Abbreviation | Full Form |
|-------------|-----------|
| API | Application Programming Interface |
| CA | Certificate Authority |
| CEL | Common Expression Language |
| CI/CD | Continuous Integration / Continuous Deployment |
| CORS | Cross-Origin Resource Sharing |
| CRD | Custom Resource Definition (Kubernetes) |
| ECDSA | Elliptic Curve Digital Signature Algorithm |
| HA | High Availability |
| HMAC | Hash-based Message Authentication Code |
| IAM | Identity and Access Management |
| JWT | JSON Web Token |
| KDF | Key Derivation Function |
| KMS | Key Management Service |
| mTLS | Mutual TLS |
| NATS | Neural Autonomic Transport System |
| OPA | Open Policy Agent |
| OTLP | OpenTelemetry Protocol |
| PII | Personally Identifiable Information |
| PKI | Public Key Infrastructure |
| RBAC | Role-Based Access Control |
| RFC | Request for Comments |
| RSA | Rivest-Shamir-Adleman (cryptographic algorithm) |
| SAN | Subject Alternative Name (in certificates) |
| SBOM | Software Bill of Materials |
| SNMP | Simple Network Management Protocol |
| SPIFFE | Secure Production Identity Framework For Everyone |
| SSH | Secure Shell |
| SSL | Secure Sockets Layer (deprecated, use TLS) |
| SVID | SPIFFE Verifiable Identity Document |
| TLS | Transport Layer Security |
| TPM | Trusted Platform Module |
| TTL | Time To Live |
| USM | User-based Security Model (SNMPv3) |
| UUID | Universally Unique Identifier |
| WASM | WebAssembly |
| WinRM | Windows Remote Management |
