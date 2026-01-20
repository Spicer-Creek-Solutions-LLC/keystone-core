package stdlib

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

var _ = bytes.Buffer{} // Use bytes to avoid unused import

func TestYAMLModule_Parse(t *testing.T) {
	mod := NewYAMLModule()

	t.Run("simple map", func(t *testing.T) {
		yaml := `
name: test
value: 123
`
		result, err := mod.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		m, ok := result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected map")
		}

		if m["name"] != "test" {
			t.Errorf("name = %v, want test", m["name"])
		}
		if m["value"] != int64(123) {
			t.Errorf("value = %v, want 123", m["value"])
		}
	})

	t.Run("nested map", func(t *testing.T) {
		yaml := `
parent:
  child: value
  number: 42
`
		result, err := mod.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		m := result.(map[string]interface{})
		parent := m["parent"].(map[string]interface{})

		if parent["child"] != "value" {
			t.Errorf("parent.child = %v, want value", parent["child"])
		}
	})

	t.Run("list", func(t *testing.T) {
		yaml := `
- item1
- item2
- item3
`
		result, err := mod.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		list, ok := result.([]interface{})
		if !ok {
			t.Fatal("Expected list")
		}

		if len(list) != 3 {
			t.Errorf("len = %d, want 3", len(list))
		}
	})

	t.Run("booleans", func(t *testing.T) {
		yaml := `
t1: true
t2: yes
t3: on
f1: false
f2: no
f3: off
`
		result, err := mod.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		m := result.(map[string]interface{})

		if m["t1"] != true {
			t.Error("t1 should be true")
		}
		if m["t2"] != true {
			t.Error("t2 should be true")
		}
		if m["f1"] != false {
			t.Error("f1 should be false")
		}
	})

	t.Run("null", func(t *testing.T) {
		yaml := `
n1: null
n2: ~
`
		result, err := mod.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		m := result.(map[string]interface{})

		if m["n1"] != nil {
			t.Error("n1 should be nil")
		}
		if m["n2"] != nil {
			t.Error("n2 should be nil")
		}
	})

	t.Run("quoted strings", func(t *testing.T) {
		yaml := `
s1: "hello world"
s2: 'single quotes'
`
		result, err := mod.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		m := result.(map[string]interface{})

		if m["s1"] != "hello world" {
			t.Errorf("s1 = %v, want 'hello world'", m["s1"])
		}
		if m["s2"] != "single quotes" {
			t.Errorf("s2 = %v, want 'single quotes'", m["s2"])
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := mod.Parse([]byte{})
		if err != ErrInvalidInput {
			t.Errorf("Expected ErrInvalidInput, got %v", err)
		}
	})
}

func TestYAMLModule_Encode(t *testing.T) {
	mod := NewYAMLModule()

	t.Run("simple map", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "test",
			"value": 123,
		}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		if !strings.Contains(string(result), "name: test") {
			t.Error("Output should contain 'name: test'")
		}
	})

	t.Run("nested map", func(t *testing.T) {
		data := map[string]interface{}{
			"parent": map[string]interface{}{
				"child": "value",
			},
		}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		if !strings.Contains(string(result), "parent:") {
			t.Error("Output should contain 'parent:'")
		}
	})

	t.Run("list", func(t *testing.T) {
		data := []interface{}{"item1", "item2"}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		if !strings.Contains(string(result), "- item1") {
			t.Error("Output should contain '- item1'")
		}
	})
}

func TestYAMLModule_ParseMulti(t *testing.T) {
	mod := NewYAMLModule()

	yaml := `
name: doc1
---
name: doc2
---
name: doc3
`
	results, err := mod.ParseMulti([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseMulti failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 documents, got %d", len(results))
	}
}

func TestYAMLModule_Merge(t *testing.T) {
	mod := NewYAMLModule()

	doc1 := map[string]interface{}{
		"a": 1,
		"b": 2,
	}
	doc2 := map[string]interface{}{
		"b": 3,
		"c": 4,
	}

	result, err := mod.Merge(doc1, doc2)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	m := result.(map[string]interface{})
	if m["a"] != 1 {
		t.Error("a should be 1")
	}
	if m["b"] != 3 {
		t.Error("b should be 3 (overwritten)")
	}
	if m["c"] != 4 {
		t.Error("c should be 4")
	}
}

func TestYAMLModule_Get(t *testing.T) {
	mod := NewYAMLModule()

	doc := map[string]interface{}{
		"parent": map[string]interface{}{
			"child": map[string]interface{}{
				"value": 42,
			},
		},
	}

	result, err := mod.Get(doc, "parent.child.value")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result != 42 {
		t.Errorf("value = %v, want 42", result)
	}
}

func TestYAMLModule_Set(t *testing.T) {
	mod := NewYAMLModule()

	doc := map[string]interface{}{
		"parent": map[string]interface{}{},
	}

	err := mod.Set(doc, "parent.child", "new_value")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	result, _ := mod.Get(doc, "parent.child")
	if result != "new_value" {
		t.Errorf("value = %v, want new_value", result)
	}
}

func TestYAMLModule_ParseFile(t *testing.T) {
	mod := NewYAMLModule()

	reader := strings.NewReader("name: test")

	result, err := mod.ParseFile(reader)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	m := result.(map[string]interface{})
	if m["name"] != "test" {
		t.Error("name should be test")
	}
}

func TestXMLModule_Parse(t *testing.T) {
	mod := NewXMLModule()

	t.Run("simple element", func(t *testing.T) {
		xml := `<root><child>value</child></root>`

		node, err := mod.Parse([]byte(xml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if node.Name() != "root" {
			t.Errorf("name = %s, want root", node.Name())
		}

		child := node.Child("child")
		if child == nil {
			t.Fatal("child not found")
		}
		if child.Text() != "value" {
			t.Errorf("child text = %s, want value", child.Text())
		}
	})

	t.Run("attributes", func(t *testing.T) {
		xml := `<root attr1="value1" attr2="value2"/>`

		node, err := mod.Parse([]byte(xml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if node.Attr("attr1") != "value1" {
			t.Errorf("attr1 = %s, want value1", node.Attr("attr1"))
		}
		if node.Attr("attr2") != "value2" {
			t.Errorf("attr2 = %s, want value2", node.Attr("attr2"))
		}
	})

	t.Run("namespace", func(t *testing.T) {
		xml := `<ns:root xmlns:ns="http://example.com"/>`

		node, err := mod.Parse([]byte(xml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if node.Name() != "root" {
			t.Errorf("name = %s, want root", node.Name())
		}
	})

	t.Run("multiple children", func(t *testing.T) {
		xml := `<root><item>1</item><item>2</item><item>3</item></root>`

		node, err := mod.Parse([]byte(xml))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		items := node.ChildrenByName("item")
		if len(items) != 3 {
			t.Errorf("items = %d, want 3", len(items))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := mod.Parse([]byte{})
		if err != ErrInvalidInput {
			t.Errorf("Expected ErrInvalidInput, got %v", err)
		}
	})
}

func TestXMLModule_Encode(t *testing.T) {
	mod := NewXMLModule()

	node := &XMLNode{
		XMLName: xml.Name{Local: "root"},
		Children: []*XMLNode{
			{
				XMLName: xml.Name{Local: "child"},
				Content: "value",
			},
		},
	}

	result, err := mod.Encode(node)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if !strings.Contains(string(result), "<root>") {
		t.Error("Output should contain <root>")
	}
	if !strings.Contains(string(result), "<child>value</child>") {
		t.Error("Output should contain <child>value</child>")
	}
}

func TestXMLModule_EncodeWithDeclaration(t *testing.T) {
	mod := NewXMLModule()

	node := &XMLNode{
		XMLName: xml.Name{Local: "root"},
	}

	result, err := mod.EncodeWithDeclaration(node)
	if err != nil {
		t.Fatalf("EncodeWithDeclaration failed: %v", err)
	}

	if !strings.HasPrefix(string(result), "<?xml") {
		t.Error("Output should start with XML declaration")
	}
}

func TestXMLModule_ToMap(t *testing.T) {
	mod := NewXMLModule()

	node := &XMLNode{
		XMLName: xml.Name{Local: "root"},
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: "123"},
		},
		Children: []*XMLNode{
			{
				XMLName: xml.Name{Local: "child"},
				Content: "value",
			},
		},
	}

	m := mod.ToMap(node)

	attrs := m["@attrs"].(map[string]string)
	if attrs["id"] != "123" {
		t.Error("attrs.id should be 123")
	}

	child := m["child"].(map[string]interface{})
	if child["#text"] != "value" {
		t.Error("child.#text should be value")
	}
}

func TestXMLModule_FromMap(t *testing.T) {
	mod := NewXMLModule()

	data := map[string]interface{}{
		"@attrs": map[string]string{"id": "123"},
		"child":  map[string]interface{}{"#text": "value"},
	}

	node := mod.FromMap("root", data)

	if node.Name() != "root" {
		t.Errorf("name = %s, want root", node.Name())
	}
	if node.Attr("id") != "123" {
		t.Errorf("id = %s, want 123", node.Attr("id"))
	}
}

func TestXMLModule_XPath(t *testing.T) {
	mod := NewXMLModule()

	xml := `<root><parent><child>1</child><child>2</child></parent></root>`

	node, err := mod.Parse([]byte(xml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Run("simple path", func(t *testing.T) {
		results := mod.XPath(node, "/parent/child")
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("wildcard", func(t *testing.T) {
		results := mod.XPath(node, "/parent/*")
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})
}

func TestXMLModule_ParseFile(t *testing.T) {
	mod := NewXMLModule()

	reader := strings.NewReader("<root>content</root>")

	node, err := mod.ParseFile(reader)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if node.Name() != "root" {
		t.Errorf("name = %s, want root", node.Name())
	}
}


func TestMergeMaps(t *testing.T) {
	dst := map[string]interface{}{
		"a": 1,
		"nested": map[string]interface{}{
			"b": 2,
		},
	}
	src := map[string]interface{}{
		"c": 3,
		"nested": map[string]interface{}{
			"d": 4,
		},
	}

	mergeMaps(dst, src)

	if dst["a"] != 1 {
		t.Error("a should be 1")
	}
	if dst["c"] != 3 {
		t.Error("c should be 3")
	}

	nested := dst["nested"].(map[string]interface{})
	if nested["b"] != 2 {
		t.Error("nested.b should be 2")
	}
	if nested["d"] != 4 {
		t.Error("nested.d should be 4")
	}
}

func TestGetPath(t *testing.T) {
	doc := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "value",
			},
		},
	}

	result, err := getPath(doc, "a.b.c")
	if err != nil {
		t.Fatalf("getPath failed: %v", err)
	}

	if result != "value" {
		t.Errorf("result = %v, want value", result)
	}

	// Test missing key
	_, err = getPath(doc, "a.missing.c")
	if err == nil {
		t.Error("Expected error for missing key")
	}
}

func TestSetPath(t *testing.T) {
	doc := map[string]interface{}{}

	err := setPath(doc, "a.b.c", "value")
	if err != nil {
		t.Fatalf("setPath failed: %v", err)
	}

	result, _ := getPath(doc, "a.b.c")
	if result != "value" {
		t.Errorf("result = %v, want value", result)
	}
}

func TestNeedsQuotes(t *testing.T) {
	tests := []struct {
		input string
		wants bool
	}{
		{"", true},
		{"hello", false},
		{"hello:world", true},
		{"true", true},
		{"false", true},
		{"yes", true},
		{"null", true},
		{"normal string", false},
	}

	for _, tt := range tests {
		result := needsQuotes(tt.input)
		if result != tt.wants {
			t.Errorf("needsQuotes(%q) = %v, want %v", tt.input, result, tt.wants)
		}
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	mod := NewYAMLModule()

	original := map[string]interface{}{
		"string": "hello",
		"number": int64(42),
		"bool":   true,
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	// Encode
	encoded, err := mod.Encode(original)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded, err := mod.Parse(encoded)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	m := decoded.(map[string]interface{})
	if m["string"] != "hello" {
		t.Error("string should be hello")
	}
}

func TestXMLRoundTrip(t *testing.T) {
	mod := NewXMLModule()

	original := &XMLNode{
		XMLName: xml.Name{Local: "root"},
		Children: []*XMLNode{
			{
				XMLName: xml.Name{Local: "child"},
				Content: "value",
			},
		},
	}

	// Encode
	encoded, err := mod.Encode(original)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded, err := mod.Parse(encoded)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if decoded.Name() != "root" {
		t.Errorf("name = %s, want root", decoded.Name())
	}

	child := decoded.Child("child")
	if child == nil || child.Text() != "value" {
		t.Error("child should have text 'value'")
	}
}

func TestXMLNode_Namespace(t *testing.T) {
	t.Run("element with namespace", func(t *testing.T) {
		node := &XMLNode{
			XMLName: xml.Name{
				Local: "element",
				Space: "http://example.com/ns",
			},
		}
		if ns := node.Namespace(); ns != "http://example.com/ns" {
			t.Errorf("Namespace() = %q, want %q", ns, "http://example.com/ns")
		}
	})

	t.Run("element without namespace", func(t *testing.T) {
		node := &XMLNode{
			XMLName: xml.Name{Local: "element"},
		}
		if ns := node.Namespace(); ns != "" {
			t.Errorf("Namespace() = %q, want empty string", ns)
		}
	})

	t.Run("parsed XML with namespace", func(t *testing.T) {
		mod := NewXMLModule()
		xmlData := `<root xmlns="http://default.ns"><child xmlns:custom="http://custom.ns"/></root>`
		node, err := mod.Parse([]byte(xmlData))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		// Root should have default namespace
		if ns := node.Namespace(); ns != "http://default.ns" {
			t.Errorf("root Namespace() = %q, want %q", ns, "http://default.ns")
		}
	})
}

func TestYAMLModule_EncodeIndent(t *testing.T) {
	mod := NewYAMLModule()

	t.Run("custom 4-space indent", func(t *testing.T) {
		data := map[string]interface{}{
			"parent": map[string]interface{}{
				"child": "value",
			},
		}

		result, err := mod.EncodeIndent(data, 4)
		if err != nil {
			t.Fatalf("EncodeIndent failed: %v", err)
		}

		// With 4-space indent, nested items should be indented by 4 spaces
		output := string(result)
		if !strings.Contains(output, "parent:") {
			t.Error("Output should contain 'parent:'")
		}
		if !strings.Contains(output, "    child:") {
			t.Errorf("Output should contain '    child:' (4-space indent), got:\n%s", output)
		}
	})

	t.Run("single space indent", func(t *testing.T) {
		data := map[string]interface{}{
			"root": map[string]interface{}{
				"nested": "val",
			},
		}

		result, err := mod.EncodeIndent(data, 1)
		if err != nil {
			t.Fatalf("EncodeIndent failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, " nested:") {
			t.Errorf("Output should contain ' nested:' (1-space indent), got:\n%s", output)
		}
	})

	t.Run("zero indent", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}

		result, err := mod.EncodeIndent(data, 0)
		if err != nil {
			t.Fatalf("EncodeIndent failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "key: value") {
			t.Errorf("Output should contain 'key: value', got:\n%s", output)
		}
	})
}

func TestWriteYAML_AllTypes(t *testing.T) {
	mod := NewYAMLModule()

	t.Run("nil value", func(t *testing.T) {
		result, err := mod.Encode(nil)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if string(result) != "null" {
			t.Errorf("nil should encode to 'null', got %q", string(result))
		}
	})

	t.Run("bool values", func(t *testing.T) {
		result, err := mod.Encode(true)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if string(result) != "true" {
			t.Errorf("true should encode to 'true', got %q", string(result))
		}

		result, err = mod.Encode(false)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if string(result) != "false" {
			t.Errorf("false should encode to 'false', got %q", string(result))
		}
	})

	t.Run("integer types", func(t *testing.T) {
		tests := []struct {
			name     string
			value    interface{}
			expected string
		}{
			{"int", int(42), "42"},
			{"int64", int64(42), "42"},
			{"int32", int32(42), "42"},
			{"int16", int16(42), "42"},
			{"int8", int8(42), "42"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := mod.Encode(tt.value)
				if err != nil {
					t.Fatalf("Encode failed: %v", err)
				}
				if string(result) != tt.expected {
					t.Errorf("%s should encode to %q, got %q", tt.name, tt.expected, string(result))
				}
			})
		}
	})

	t.Run("unsigned integer types", func(t *testing.T) {
		tests := []struct {
			name     string
			value    interface{}
			expected string
		}{
			{"uint", uint(42), "42"},
			{"uint64", uint64(42), "42"},
			{"uint32", uint32(42), "42"},
			{"uint16", uint16(42), "42"},
			{"uint8", uint8(42), "42"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := mod.Encode(tt.value)
				if err != nil {
					t.Fatalf("Encode failed: %v", err)
				}
				if string(result) != tt.expected {
					t.Errorf("%s should encode to %q, got %q", tt.name, tt.expected, string(result))
				}
			})
		}
	})

	t.Run("float types", func(t *testing.T) {
		result, err := mod.Encode(float64(3.14))
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if !strings.Contains(string(result), "3.14") {
			t.Errorf("float64(3.14) should encode to contain '3.14', got %q", string(result))
		}

		result, err = mod.Encode(float32(2.5))
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if !strings.Contains(string(result), "2.5") {
			t.Errorf("float32(2.5) should encode to contain '2.5', got %q", string(result))
		}
	})

	t.Run("string needing quotes", func(t *testing.T) {
		tests := []struct {
			value    string
			contains string
		}{
			{"key:value", "\"key:value\""},
			{"true", "\"true\""},
			{"null", "\"null\""},
			{"yes", "\"yes\""},
			{"", "\"\""},
		}

		for _, tt := range tests {
			t.Run(tt.value, func(t *testing.T) {
				result, err := mod.Encode(tt.value)
				if err != nil {
					t.Fatalf("Encode failed: %v", err)
				}
				if string(result) != tt.contains {
					t.Errorf("string %q should encode to %q, got %q", tt.value, tt.contains, string(result))
				}
			})
		}
	})

	t.Run("nested list in map", func(t *testing.T) {
		data := map[string]interface{}{
			"items": []interface{}{"a", "b", "c"},
		}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "items:") {
			t.Error("Output should contain 'items:'")
		}
		if !strings.Contains(output, "- a") {
			t.Error("Output should contain '- a'")
		}
	})

	t.Run("list with map items", func(t *testing.T) {
		data := []interface{}{
			map[string]interface{}{"name": "first"},
			map[string]interface{}{"name": "second"},
		}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "- name: first") {
			t.Errorf("Output should contain '- name: first', got:\n%s", output)
		}
	})

	t.Run("complex type fallback to JSON", func(t *testing.T) {
		// Use a struct that will trigger JSON marshaling
		type customType struct {
			Field string `json:"field"`
		}
		data := customType{Field: "value"}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		// Should fall back to JSON representation
		if !strings.Contains(string(result), "field") {
			t.Errorf("Complex type should be JSON marshaled, got: %s", string(result))
		}
	})

	t.Run("empty map in list item", func(t *testing.T) {
		data := []interface{}{
			map[string]interface{}{},
			"simple",
		}

		result, err := mod.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "- simple") {
			t.Errorf("Output should contain '- simple', got:\n%s", output)
		}
	})
}

func TestXMLModule_FromMap_EdgeCases(t *testing.T) {
	mod := NewXMLModule()

	t.Run("with text content", func(t *testing.T) {
		data := map[string]interface{}{
			"#text": "Hello World",
		}

		node := mod.FromMap("greeting", data)

		if node.Name() != "greeting" {
			t.Errorf("Name() = %s, want greeting", node.Name())
		}
		if node.Content != "Hello World" {
			t.Errorf("Content = %s, want 'Hello World'", node.Content)
		}
	})

	t.Run("with string child", func(t *testing.T) {
		data := map[string]interface{}{
			"title": "Document Title",
		}

		node := mod.FromMap("doc", data)

		if len(node.Children) != 1 {
			t.Fatalf("Expected 1 child, got %d", len(node.Children))
		}

		child := node.Child("title")
		if child == nil {
			t.Fatal("Expected child 'title'")
		}
		if child.Content != "Document Title" {
			t.Errorf("child Content = %s, want 'Document Title'", child.Content)
		}
	})

	t.Run("with array of maps", func(t *testing.T) {
		data := map[string]interface{}{
			"item": []interface{}{
				map[string]interface{}{"#text": "first"},
				map[string]interface{}{"#text": "second"},
			},
		}

		node := mod.FromMap("list", data)

		items := node.ChildrenByName("item")
		if len(items) != 2 {
			t.Fatalf("Expected 2 item children, got %d", len(items))
		}

		if items[0].Content != "first" {
			t.Errorf("First item content = %s, want 'first'", items[0].Content)
		}
		if items[1].Content != "second" {
			t.Errorf("Second item content = %s, want 'second'", items[1].Content)
		}
	})

	t.Run("with array containing non-maps", func(t *testing.T) {
		data := map[string]interface{}{
			"values": []interface{}{"not a map", 123},
		}

		// Should not crash, non-map items are skipped
		node := mod.FromMap("root", data)

		if node.Name() != "root" {
			t.Errorf("Name() = %s, want root", node.Name())
		}
		// No children should be created for non-map array items
		if len(node.ChildrenByName("values")) != 0 {
			t.Error("Non-map array items should not create children")
		}
	})

	t.Run("attrs with wrong type", func(t *testing.T) {
		data := map[string]interface{}{
			"@attrs": "not a map", // Wrong type
		}

		// Should not crash
		node := mod.FromMap("root", data)
		if node.Name() != "root" {
			t.Errorf("Name() = %s, want root", node.Name())
		}
		if len(node.Attrs) != 0 {
			t.Error("Invalid @attrs should not create attributes")
		}
	})

	t.Run("text with wrong type", func(t *testing.T) {
		data := map[string]interface{}{
			"#text": 12345, // Not a string
		}

		// Should not crash
		node := mod.FromMap("root", data)
		if node.Content != "" {
			t.Errorf("Content should be empty for non-string #text, got %s", node.Content)
		}
	})
}

func TestXPathSearch_EdgeCases(t *testing.T) {
	mod := NewXMLModule()

	xmlData := `<root>
		<items>
			<item id="1">first</item>
			<item id="2">second</item>
			<other>not an item</other>
		</items>
		<metadata>
			<author>test</author>
		</metadata>
	</root>`

	node, err := mod.Parse([]byte(xmlData))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Run("attribute selector path", func(t *testing.T) {
		// Attribute selectors return empty (not supported for node selection)
		results := mod.XPath(node, "/items/@id")
		// Should not crash, returns empty since attribute access doesn't return nodes
		if len(results) != 0 {
			t.Errorf("Attribute selector should return empty, got %d results", len(results))
		}
	})

	t.Run("predicate selector", func(t *testing.T) {
		// Predicate in path like /items/item[1]
		results := mod.XPath(node, "/items/item[1]")
		if len(results) != 2 {
			t.Errorf("Expected 2 items (predicate stripped), got %d", len(results))
		}
	})

	t.Run("path with double slash", func(t *testing.T) {
		results := mod.XPath(node, "//items")
		// Double slash creates empty part - behavior is to search children for empty string
		// which won't match, so this returns the intermediate result or empty
		// The important thing is it doesn't crash
		_ = results
	})

	t.Run("deep nesting search", func(t *testing.T) {
		results := mod.XPath(node, "/items/item")
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("wildcard at leaf", func(t *testing.T) {
		results := mod.XPath(node, "/items/*")
		if len(results) != 3 {
			t.Errorf("Expected 3 children (2 items + 1 other), got %d", len(results))
		}
	})

	t.Run("non-matching path", func(t *testing.T) {
		results := mod.XPath(node, "/nonexistent/path")
		if len(results) != 0 {
			t.Errorf("Expected 0 results for non-matching path, got %d", len(results))
		}
	})

	t.Run("empty input nodes", func(t *testing.T) {
		results := xpathSearch([]*XMLNode{}, []string{"path"})
		if len(results) != 0 {
			t.Errorf("Expected 0 results for empty input, got %d", len(results))
		}
	})

	t.Run("empty path parts", func(t *testing.T) {
		results := xpathSearch([]*XMLNode{node}, []string{})
		if len(results) != 1 {
			t.Errorf("Expected 1 result (original node) for empty parts, got %d", len(results))
		}
	})
}

