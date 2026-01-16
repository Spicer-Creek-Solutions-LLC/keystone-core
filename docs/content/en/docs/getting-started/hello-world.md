---
title: "Hello World"
linkTitle: "Hello World"
weight: 5
description: >
  Apply your first minimal state file
---

## Overview

This example applies a single `file` state to create a hello world file on an agent.

Prerequisite: complete the [Quick Start](../quick-start/) so you have a running
control plane and an agent ID to target.

## Use the Example State

Copy the example from `examples/states/hello-world.yaml` into your working directory:

```bash
cp examples/states/hello-world.yaml ./hello-world.yaml
```

Apply the state to your agent:

```bash
kscorectl state apply ./hello-world.yaml
```

This applies locally on the host running `kscorectl`.

Verify the result:

```bash
kscorectl exec run --target "agent_id:my-first-agent" -- cat /tmp/hello-from-kscore.txt
```

Expected output:

```
Hello from Keystone Core!
```
