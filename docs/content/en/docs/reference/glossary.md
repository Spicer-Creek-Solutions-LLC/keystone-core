---
title: "Glossary"
weight: 14
description: >
  Terminology reference for Keystone Core concepts
---

## Terms

**Agent**: Lightweight daemon installed on managed nodes to execute commands and apply state.

**Blueprint**: A reusable, versioned bundle of states and parameters for deploying a system.

**Control Plane**: The set of server components that manage state, events, and orchestration.

**Embedded Mode**: A deployment mode that uses in-process NATS and SQLite for zero-dependency setup.

**Event**: A structured message emitted by agents or control plane services for automation or audit.

**Leaf Node**: A NATS node that connects to another NATS server to extend a mesh across networks.

**Module**: A programmable extension (Starlark or WASM) that adds new state types or behavior.

**Reactor**: An event-driven automation rule that triggers actions based on events.

**State**: A declarative description of desired system configuration.

**State Run**: An execution of states that produces results and drift information.
