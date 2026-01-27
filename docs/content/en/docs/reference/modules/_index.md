---
title: Module Reference
description: Auto-generated documentation for plugin modules
weight: 50
---

This section contains auto-generated documentation for Keystone Core plugin modules.

## Available Modules

- [DNS Records](dns/) - Manage DNS records across multiple providers
- [Standard Library Modules](stdlib/) - Built-in YAML, XML, JSON, and encoding modules
- [Resource Management](resources/) - Module resource limits and usage tracking

## Generating Documentation

Module documentation is auto-generated from Go source code using the `moddoc` tool:

```bash
# Generate all module docs
make docs-modules

# Or run directly
./tools/moddoc/generate.sh
```

The tool extracts documentation from:
- Package comments
- Type definitions and their doc comments
- Method signatures and documentation
- Field documentation
