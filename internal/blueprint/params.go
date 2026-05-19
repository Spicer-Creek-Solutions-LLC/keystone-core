package blueprint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// ErrUnknownParam is returned when an input names a parameter the
// manifest does not declare.
var ErrUnknownParam = errors.New("blueprint: unknown parameter")

// ErrParamCoercion is returned when a string input cannot be coerced
// to its declared type. The error always names the parameter and the
// offending value — never silently zero-coerced (§4.17 gotcha).
var ErrParamCoercion = errors.New("blueprint: parameter coercion failed")

// ErrParamValidation wraps a JSON Schema validation failure.
var ErrParamValidation = errors.New("blueprint: parameter validation failed")

const paramSchemaURL = "mem://keystone/blueprint/params.json"

// ResolvedParams is the output of ResolveParams: every parameter's
// resolved (coerced + defaulted) value plus the names of parameters
// whose Source is "secret". The secret values are NOT fetched here —
// the executor (Epic 15 task 5) resolves them via the SecretBroker.
type ResolvedParams struct {
	Values map[string]any
	Secret []string
}

// Redacted returns a copy of Values safe to log: every sensitive or
// secret-sourced parameter's value is replaced with "***".
func (r ResolvedParams) Redacted(m *Manifest) map[string]any {
	out := make(map[string]any, len(r.Values))
	for k, v := range r.Values {
		if spec, ok := m.Parameters[k]; ok && (spec.Sensitive || spec.Source == SourceSecret) {
			out[k] = "***"
			continue
		}
		out[k] = v
	}
	return out
}

// ResolveParams coerces string-shaped inputs to each parameter's
// declared type, applies defaults for absent parameters, then
// validates the assembled object against the JSON Schema built from
// the parameters block. Unknown input keys are rejected.
//
// Secret-sourced parameters with no input keep their (possibly nil)
// declared default here; the executor substitutes the real value.
func (m *Manifest) ResolveParams(inputs map[string]string) (ResolvedParams, error) {
	for name := range inputs {
		if _, ok := m.Parameters[name]; !ok {
			return ResolvedParams{}, fmt.Errorf("%w: %q", ErrUnknownParam, name)
		}
	}

	values := make(map[string]any, len(m.Parameters))
	var secret []string
	for name, spec := range m.Parameters {
		if spec.Source == SourceSecret {
			secret = append(secret, name)
		}
		raw, given := inputs[name]
		if !given {
			if spec.Default != nil {
				values[name] = spec.Default
			}
			continue
		}
		v, err := coerce(name, spec.Type, raw)
		if err != nil {
			return ResolvedParams{}, err
		}
		values[name] = v
	}
	sort.Strings(secret)

	sch, err := m.compileParamSchema()
	if err != nil {
		return ResolvedParams{}, fmt.Errorf("blueprint: build param schema: %w", err)
	}
	instance, err := toJSONValue(values)
	if err != nil {
		return ResolvedParams{}, fmt.Errorf("blueprint: encode params: %w", err)
	}
	if err := sch.Validate(instance); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return ResolvedParams{}, fmt.Errorf("%w: %s", ErrParamValidation, ve)
		}
		return ResolvedParams{}, fmt.Errorf("%w: %w", ErrParamValidation, err)
	}

	return ResolvedParams{Values: values, Secret: secret}, nil
}

// coerce converts a string input to the parameter's declared type.
func coerce(name, typ, raw string) (any, error) {
	switch typ {
	case TypeString:
		return raw, nil
	case TypeInteger:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parameter %q: %q is not an integer", ErrParamCoercion, name, raw)
		}
		return n, nil
	case TypeNumber:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parameter %q: %q is not a number", ErrParamCoercion, name, raw)
		}
		return f, nil
	case TypeBoolean:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: parameter %q: %q is not a boolean", ErrParamCoercion, name, raw)
		}
		return b, nil
	case TypeArray, TypeObject:
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("%w: parameter %q: %q is not valid JSON %s", ErrParamCoercion, name, raw, typ)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("%w: parameter %q: unsupported type %q", ErrParamCoercion, name, typ)
	}
}

// compileParamSchema assembles a JSON Schema 2020-12 object from the
// parameters block and compiles it. Sensitive/Source are Keystone
// annotations and are intentionally not emitted into the schema.
func (m *Manifest) compileParamSchema() (*jsonschema.Schema, error) {
	props := make(map[string]any, len(m.Parameters))
	var required []string
	for name, spec := range m.Parameters {
		p := map[string]any{}
		if spec.Type != "" {
			p["type"] = spec.Type
		}
		if len(spec.Enum) > 0 {
			p["enum"] = spec.Enum
		}
		if spec.Min != nil {
			p["minimum"] = *spec.Min
		}
		if spec.Max != nil {
			p["maximum"] = *spec.Max
		}
		if spec.Pattern != "" {
			p["pattern"] = spec.Pattern
		}
		props[name] = p
		if spec.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)

	doc := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		doc["required"] = required
	}

	canonical, err := toJSONValue(doc)
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(paramSchemaURL, canonical); err != nil {
		return nil, err
	}
	return c.Compile(paramSchemaURL)
}

// toJSONValue round-trips v through encoding/json so it becomes the
// canonical decoded shape (map[string]any / []any / float64 / …) that
// the schema compiler and validator expect.
func toJSONValue(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}
