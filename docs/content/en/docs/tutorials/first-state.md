---
title: "Your First State"
weight: 10
description: >
  Create and apply your first state declaration to manage a configuration file.
---

## Overview

In this tutorial, you'll learn how to:
- Create a state declaration file
- Apply states to managed nodes
- Verify the applied configuration
- Understand state idempotency

**Time**: 10 minutes

## Prerequisites

- Keystone Core control plane running
- At least one agent connected
- `kscorectl` CLI configured

Verify your setup:
```bash
kscorectl agent list
```

You should see at least one agent in the output.

## Step 1: Create a State File

Create a file named `hello-state.yaml`:

```yaml
# hello-state.yaml
# A simple state that creates a greeting file

states:
  file:
    - id: /tmp/hello-keystone.txt
      state: present
      parameters:
        contents: |
          Hello from Keystone Core!

          This file was created by declarative state management.
          Timestamp: {{ .facts.timestamp }}
        mode: "0644"
```

This state:
- Creates a file at `/tmp/hello-keystone.txt`
- Sets the content with a template that includes a timestamp
- Sets file permissions to `0644`

## Step 2: Check What Will Change

Before applying, preview the changes:

```bash
kscorectl state check hello-state.yaml
```

Output:
```
Checking state: hello-state.yaml

Target: all agents

[agent-001] Checking...
  /tmp/hello-keystone.txt:
    State: absent → present
    Status: will create

Summary: 1 change(s) pending
```

The `check` command shows what would change without actually making modifications.

## Step 3: Apply the State

Apply the state to all agents:

```bash
kscorectl state apply hello-state.yaml
```

Output:
```
Applying state: hello-state.yaml

Target: all agents

[agent-001] Applying...
  ✓ /tmp/hello-keystone.txt: created

Summary:
  Total:     1
  Succeeded: 1
  Changed:   1
  Failed:    0
```

## Step 4: Verify the Result

Verify the file was created:

```bash
kscorectl exec run "*" -- cat /tmp/hello-keystone.txt
```

Output:
```
[agent-001]
Hello from Keystone Core!

This file was created by declarative state management.
Timestamp: 2024-01-15T10:30:00Z
```

## Step 5: Understanding Idempotency

Run the same apply command again:

```bash
kscorectl state apply hello-state.yaml
```

Output:
```
Applying state: hello-state.yaml

Target: all agents

[agent-001] Applying...
  - /tmp/hello-keystone.txt: unchanged

Summary:
  Total:     1
  Succeeded: 1
  Changed:   0
  Failed:    0
```

Notice that `Changed: 0`. This is **idempotency** - the state is already correct, so no changes are made. You can safely run state applications repeatedly without causing unintended side effects.

## Step 6: Modify the State

Update the file content:

```yaml
# hello-state.yaml (updated)
states:
  file:
    - id: /tmp/hello-keystone.txt
      state: present
      parameters:
        contents: |
          Hello from Keystone Core!

          This file was updated by declarative state management.
          Version: 2
          Timestamp: {{ .facts.timestamp }}
        mode: "0644"
```

Check and apply:

```bash
kscorectl state check hello-state.yaml
```

Output:
```
[agent-001] Checking...
  /tmp/hello-keystone.txt:
    State: present → present
    Status: will update (contents changed)
```

```bash
kscorectl state apply hello-state.yaml
```

Only the content is updated; the file isn't recreated.

## Step 7: Remove the State

To remove the file, change the state to `absent`:

```yaml
# hello-state.yaml (cleanup)
states:
  file:
    - id: /tmp/hello-keystone.txt
      state: absent
```

Apply to remove:

```bash
kscorectl state apply hello-state.yaml
```

## Complete Example with Multiple Resources

Here's a more complete example managing multiple resources:

```yaml
# complete-example.yaml
metadata:
  description: Complete example with multiple resources

vars:
  app_name: myapp
  app_user: appuser

states:
  # Create application user
  user:
    - id: {{ .vars.app_user }}
      state: present
      parameters:
        shell: /bin/bash
        home: /home/{{ .vars.app_user }}

  # Create application directory
  file:
    - id: /opt/{{ .vars.app_name }}
      state: directory
      parameters:
        owner: {{ .vars.app_user }}
        mode: "0755"
      require:
        - user: {{ .vars.app_user }}

    - id: /opt/{{ .vars.app_name }}/config.yaml
      state: present
      parameters:
        contents: |
          app:
            name: {{ .vars.app_name }}
            environment: {{ .facts.environment | default "development" }}
        owner: {{ .vars.app_user }}
        mode: "0640"
      require:
        - file: /opt/{{ .vars.app_name }}
```

This example demonstrates:
- **Variables**: Reusable values with `vars`
- **Templates**: Dynamic content with `{{ }}`
- **Dependencies**: `require` ensures proper ordering
- **Multiple resources**: Users, directories, and files

## Next Steps

- [Remote Execution Basics](/docs/tutorials/remote-execution/) - Run commands on agents
- [Multi-Tier Web Application](/docs/scenarios/multi-tier-webapp/) - A real-world example
- [State Management Concepts](/docs/concepts/state-management/) - Deep dive into states
