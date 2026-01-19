#!/bin/bash
#
# Generate module documentation from Go source code annotations.
#
# Usage:
#   ./generate.sh              # Generate all module docs
#   ./generate.sh stdlib       # Generate stdlib docs only
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCS_DIR="$ROOT_DIR/docs/content/en/docs/reference/modules"

# Build the moddoc tool
echo "Building moddoc..."
go build -o "$SCRIPT_DIR/moddoc" "$SCRIPT_DIR"

# Create output directory
mkdir -p "$DOCS_DIR"

generate_stdlib() {
    echo "Generating stdlib documentation..."
    "$SCRIPT_DIR/moddoc" \
        -t "Standard Library Modules" \
        -o "$DOCS_DIR/stdlib.md" \
        "$ROOT_DIR/pkg/plugin/stdlib"

    # Add frontmatter
    tmpfile=$(mktemp)
    cat > "$tmpfile" << 'EOF'
---
title: Standard Library Modules
description: Built-in modules for YAML, XML, and other common operations
weight: 10
---

EOF
    cat "$DOCS_DIR/stdlib.md" >> "$tmpfile"
    mv "$tmpfile" "$DOCS_DIR/stdlib.md"

    echo "Generated: $DOCS_DIR/stdlib.md"
}

generate_resources() {
    echo "Generating resource limits documentation..."
    "$SCRIPT_DIR/moddoc" \
        -t "Resource Management" \
        -p "" \
        -o "$DOCS_DIR/resources.md" \
        "$ROOT_DIR/pkg/plugin/resources"

    # Add frontmatter
    tmpfile=$(mktemp)
    cat > "$tmpfile" << 'EOF'
---
title: Resource Management
description: Module resource limits and usage tracking
weight: 20
---

EOF
    cat "$DOCS_DIR/resources.md" >> "$tmpfile"
    mv "$tmpfile" "$DOCS_DIR/resources.md"

    echo "Generated: $DOCS_DIR/resources.md"
}

generate_index() {
    echo "Generating index..."
    cat > "$DOCS_DIR/_index.md" << 'EOF'
---
title: Module Reference
description: Auto-generated documentation for plugin modules
weight: 50
---

This section contains auto-generated documentation for Keystone Core plugin modules.

## Available Modules

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
EOF
    echo "Generated: $DOCS_DIR/_index.md"
}

case "${1:-all}" in
    stdlib)
        generate_stdlib
        ;;
    resources)
        generate_resources
        ;;
    all)
        generate_index
        generate_stdlib
        generate_resources
        ;;
    *)
        echo "Usage: $0 [stdlib|resources|all]"
        exit 1
        ;;
esac

echo "Documentation generation complete!"
