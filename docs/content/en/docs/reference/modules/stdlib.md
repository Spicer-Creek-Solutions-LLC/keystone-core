---
title: Standard Library Modules
description: Built-in modules for YAML, XML, and other common operations
weight: 10
---

Package stdlib provides standard library modules for plugins.

**Import:** `github.com/shawnbutts/keystone-core/pkg/plugin/stdlib`

## Contents

- [XMLModule](#xmlmodule)
- [YAMLModule](#yamlmodule)

## Variables

### ErrInvalidInput

Errors returned by encoding modules.

**Type:** ``

### ErrParseError

Errors returned by encoding modules.

**Type:** ``

### ErrEncodeError

Errors returned by encoding modules.

**Type:** ``

### ErrUnsupportedOp

Errors returned by encoding modules.

**Type:** ``

## XMLModule

XMLModule provides XML parsing and encoding capabilities.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `StrictMode` | `bool` | StrictMode enables strict XML parsing. |
| `PreserveWhitespace` | `bool` | PreserveWhitespace preserves whitespace in text content. |

### Methods

#### Encode

```go
func (m *XMLModule) Encode(node *XMLNode) ([]byte, error)
```

Encode encodes an XMLNode to bytes.

**Parameters:**

- `node` (`*XMLNode`)

**Returns:**

- `[]byte`
- `error`

#### EncodeWithDeclaration

```go
func (m *XMLModule) EncodeWithDeclaration(node *XMLNode) ([]byte, error)
```

EncodeWithDeclaration includes XML declaration.

**Parameters:**

- `node` (`*XMLNode`)

**Returns:**

- `[]byte`
- `error`

#### FromMap

```go
func (m *XMLModule) FromMap(name string, data map[string]interface{}) *XMLNode
```

FromMap creates an XMLNode from a map.

**Parameters:**

- `name` (`string`)
- `data` (`map[string]interface{}`)

**Returns:**

- `*XMLNode`

#### Parse

```go
func (m *XMLModule) Parse(data []byte) (*XMLNode, error)
```

Parse parses XML content into an XMLNode.

**Parameters:**

- `data` (`[]byte`)

**Returns:**

- `*XMLNode`
- `error`

#### ParseFile

```go
func (m *XMLModule) ParseFile(r io.Reader) (*XMLNode, error)
```

ParseFile parses XML from a reader.

**Parameters:**

- `r` (`io.Reader`)

**Returns:**

- `*XMLNode`
- `error`

#### ToMap

```go
func (m *XMLModule) ToMap(node *XMLNode) map[string]interface{}
```

ToMap converts an XMLNode to a map.

**Parameters:**

- `node` (`*XMLNode`)

**Returns:**

- `map[string]interface{}`

#### XPath

```go
func (m *XMLModule) XPath(node *XMLNode, path string) []*XMLNode
```

XPath performs a simple XPath-like query.

**Parameters:**

- `node` (`*XMLNode`)
- `path` (`string`)

**Returns:**

- `[]*XMLNode`

---

## YAMLModule

YAMLModule provides YAML parsing and encoding capabilities.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `StrictMode` | `bool` | StrictMode enables strict YAML parsing. |
| `MaxDepth` | `int` | MaxDepth limits nesting depth. |

### Methods

#### Encode

```go
func (m *YAMLModule) Encode(v interface{}) ([]byte, error)
```

Encode encodes a value to YAML.

**Parameters:**

- `v` (`interface{}`)

**Returns:**

- `[]byte`
- `error`

#### EncodeIndent

```go
func (m *YAMLModule) EncodeIndent(v interface{}, indent int) ([]byte, error)
```

EncodeIndent encodes with custom indentation.

**Parameters:**

- `v` (`interface{}`)
- `indent` (`int`)

**Returns:**

- `[]byte`
- `error`

#### Get

```go
func (m *YAMLModule) Get(doc interface{}, path string) (interface{}, error)
```

Get gets a value by path (e.g., "foo.bar.baz").

**Parameters:**

- `doc` (`interface{}`)
- `path` (`string`)

**Returns:**

- `interface{}`
- `error`

#### Merge

```go
func (m *YAMLModule) Merge(docs ...interface{}) (interface{}, error)
```

Merge merges multiple YAML documents.

**Parameters:**

- `docs` (`...interface{}`)

**Returns:**

- `interface{}`
- `error`

#### Parse

```go
func (m *YAMLModule) Parse(data []byte) (interface{}, error)
```

Parse parses YAML content into a map or slice.

**Parameters:**

- `data` (`[]byte`)

**Returns:**

- `interface{}`
- `error`

#### ParseFile

```go
func (m *YAMLModule) ParseFile(r io.Reader) (interface{}, error)
```

ParseFile parses YAML from a reader.

**Parameters:**

- `r` (`io.Reader`)

**Returns:**

- `interface{}`
- `error`

#### ParseMulti

```go
func (m *YAMLModule) ParseMulti(data []byte) ([]interface{}, error)
```

ParseMulti parses multiple YAML documents.

**Parameters:**

- `data` (`[]byte`)

**Returns:**

- `[]interface{}`
- `error`

#### Set

```go
func (m *YAMLModule) Set(doc interface{}, path string, value interface{}) error
```

Set sets a value by path.

**Parameters:**

- `doc` (`interface{}`)
- `path` (`string`)
- `value` (`interface{}`)

**Returns:**

- `error`

---

## Functions

### collectBlock

```go
func collectBlock(lines []string, minIndent int) []string
```

### countIndent

```go
func countIndent(line string) int
```

### encodeYAML

```go
func encodeYAML(v interface{}) ([]byte, error)
```

### encodeYAMLIndent

```go
func encodeYAMLIndent(v interface{}, indent int) ([]byte, error)
```

### getPath

```go
func getPath(doc interface{}, path string) (interface{}, error)
```

### mergeMaps

```go
func mergeMaps(dst, src map[string]interface{})
```

### needsQuotes

```go
func needsQuotes(s string) bool
```

### parseValue

```go
func parseValue(s string) interface{}
```

### parseYAML

```go
func parseYAML(data []byte, maxDepth int) (interface{}, error)
```

Simple YAML parser (handles basic YAML without external dependencies)

### parseYAMLLines

```go
func parseYAMLLines(lines []string, startIndent, maxDepth, depth int) (interface{}, error)
```

### setParents

```go
func setParents(node *XMLNode, parent *XMLNode)
```

### setPath

```go
func setPath(doc interface{}, path string, value interface{}) error
```

### writeYAML

```go
func writeYAML(w io.Writer, v interface{}, level, indent int) error
```

---

*Generated by moddoc*
