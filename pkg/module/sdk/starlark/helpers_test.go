package starlark

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestConvertToGo(t *testing.T) {
	tests := []struct {
		name     string
		input    starlark.Value
		expected interface{}
	}{
		{
			name:     "none",
			input:    starlark.None,
			expected: nil,
		},
		{
			name:     "bool true",
			input:    starlark.Bool(true),
			expected: true,
		},
		{
			name:     "bool false",
			input:    starlark.Bool(false),
			expected: false,
		},
		{
			name:     "int",
			input:    starlark.MakeInt(42),
			expected: int64(42),
		},
		{
			name:     "float",
			input:    starlark.Float(3.14),
			expected: float64(3.14),
		},
		{
			name:     "string",
			input:    starlark.String("hello"),
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToGo(tt.input)
			if err != nil {
				t.Fatalf("ConvertToGo() error = %v", err)
			}

			if result != tt.expected {
				t.Errorf("ConvertToGo() = %v (%T), want %v (%T)", result, result, tt.expected, tt.expected)
			}
		})
	}
}

func TestConvertToGo_List(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.MakeInt(1),
		starlark.MakeInt(2),
		starlark.MakeInt(3),
	})

	result, err := ConvertToGo(list)
	if err != nil {
		t.Fatalf("ConvertToGo() error = %v", err)
	}

	goList, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Result is not []interface{}, got %T", result)
	}

	if len(goList) != 3 {
		t.Errorf("List length = %d, want 3", len(goList))
	}

	expected := []int64{1, 2, 3}
	for i, val := range goList {
		intVal, ok := val.(int64)
		if !ok {
			t.Errorf("List item %d is not int64, got %T", i, val)
			continue
		}
		if intVal != expected[i] {
			t.Errorf("List item %d = %d, want %d", i, intVal, expected[i])
		}
	}
}

func TestConvertToGo_Dict(t *testing.T) {
	dict := starlark.NewDict(2)
	dict.SetKey(starlark.String("name"), starlark.String("test"))
	dict.SetKey(starlark.String("age"), starlark.MakeInt(42))

	result, err := ConvertToGo(dict)
	if err != nil {
		t.Fatalf("ConvertToGo() error = %v", err)
	}

	goDict, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result is not map[string]interface{}, got %T", result)
	}

	if len(goDict) != 2 {
		t.Errorf("Dict length = %d, want 2", len(goDict))
	}

	if name, ok := goDict["name"].(string); !ok || name != "test" {
		t.Errorf("Dict['name'] = %v, want 'test'", goDict["name"])
	}

	if age, ok := goDict["age"].(int64); !ok || age != 42 {
		t.Errorf("Dict['age'] = %v, want 42", goDict["age"])
	}
}

func TestConvertFromGo(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		check    func(starlark.Value) bool
		typeName string
	}{
		{
			name:  "nil",
			input: nil,
			check: func(v starlark.Value) bool {
				_, ok := v.(starlark.NoneType)
				return ok
			},
			typeName: "NoneType",
		},
		{
			name:  "bool true",
			input: true,
			check: func(v starlark.Value) bool {
				b, ok := v.(starlark.Bool)
				return ok && bool(b) == true
			},
			typeName: "Bool",
		},
		{
			name:  "int",
			input: 42,
			check: func(v starlark.Value) bool {
				i, ok := v.(starlark.Int)
				if !ok {
					return false
				}
				val, _ := i.Int64()
				return val == 42
			},
			typeName: "Int",
		},
		{
			name:  "float64",
			input: 3.14,
			check: func(v starlark.Value) bool {
				f, ok := v.(starlark.Float)
				return ok && float64(f) == 3.14
			},
			typeName: "Float",
		},
		{
			name:  "string",
			input: "hello",
			check: func(v starlark.Value) bool {
				s, ok := v.(starlark.String)
				return ok && string(s) == "hello"
			},
			typeName: "String",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertFromGo(tt.input)
			if err != nil {
				t.Fatalf("ConvertFromGo() error = %v", err)
			}

			if !tt.check(result) {
				t.Errorf("ConvertFromGo() returned wrong type or value: %v (type %T)", result, result)
			}
		})
	}
}

func TestConvertFromGo_Slice(t *testing.T) {
	input := []interface{}{1, 2, 3}

	result, err := ConvertFromGo(input)
	if err != nil {
		t.Fatalf("ConvertFromGo() error = %v", err)
	}

	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatalf("Result is not *starlark.List, got %T", result)
	}

	if list.Len() != 3 {
		t.Errorf("List length = %d, want 3", list.Len())
	}
}

func TestConvertFromGo_Map(t *testing.T) {
	input := map[string]interface{}{
		"name": "test",
		"age":  42,
	}

	result, err := ConvertFromGo(input)
	if err != nil {
		t.Fatalf("ConvertFromGo() error = %v", err)
	}

	dict, ok := result.(*starlark.Dict)
	if !ok {
		t.Fatalf("Result is not *starlark.Dict, got %T", result)
	}

	if dict.Len() != 2 {
		t.Errorf("Dict length = %d, want 2", dict.Len())
	}

	name, found, err := dict.Get(starlark.String("name"))
	if err != nil || !found {
		t.Errorf("Failed to get 'name' from dict")
	} else if s, ok := name.(starlark.String); !ok || string(s) != "test" {
		t.Errorf("Dict['name'] = %v, want 'test'", name)
	}
}

func TestStringValue(t *testing.T) {
	val := starlark.String("hello")
	result, err := StringValue(val, "test")
	if err != nil {
		t.Fatalf("StringValue() error = %v", err)
	}
	if result != "hello" {
		t.Errorf("StringValue() = %s, want 'hello'", result)
	}

	// Test error case
	_, err = StringValue(starlark.MakeInt(42), "test")
	if err == nil {
		t.Error("StringValue() should error for non-string value")
	}
}

func TestIntValue(t *testing.T) {
	val := starlark.MakeInt(42)
	result, err := IntValue(val, "test")
	if err != nil {
		t.Fatalf("IntValue() error = %v", err)
	}
	if result != 42 {
		t.Errorf("IntValue() = %d, want 42", result)
	}

	// Test error case
	_, err = IntValue(starlark.String("hello"), "test")
	if err == nil {
		t.Error("IntValue() should error for non-int value")
	}
}

func TestBoolValue(t *testing.T) {
	val := starlark.Bool(true)
	result, err := BoolValue(val, "test")
	if err != nil {
		t.Fatalf("BoolValue() error = %v", err)
	}
	if result != true {
		t.Errorf("BoolValue() = %v, want true", result)
	}

	// Test error case
	_, err = BoolValue(starlark.String("hello"), "test")
	if err == nil {
		t.Error("BoolValue() should error for non-bool value")
	}
}
