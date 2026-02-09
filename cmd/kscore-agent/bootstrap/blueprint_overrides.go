package bootstrap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	"gopkg.in/yaml.v3"
)

func applyBlueprintOverrides(opts *Options) error {
	if len(opts.BlueprintParamArgs) == 0 && len(opts.BlueprintFeatureArgs) == 0 && len(opts.BlueprintEntrypointArgs) == 0 {
		return nil
	}

	if opts.BlueprintParams == nil {
		opts.BlueprintParams = make(map[string]map[string]interface{})
	}
	if opts.BlueprintFeatures == nil {
		opts.BlueprintFeatures = make(map[string]map[string]bool)
	}
	if opts.BlueprintEntrypoints == nil {
		opts.BlueprintEntrypoints = make(map[string]string)
	}

	for _, spec := range opts.BlueprintParamArgs {
		ref, key, value, err := parseBlueprintParamSpec(spec)
		if err != nil {
			return err
		}
		if opts.BlueprintParams[ref] == nil {
			opts.BlueprintParams[ref] = make(map[string]interface{})
		}
		opts.BlueprintParams[ref][key] = value
	}

	for _, spec := range opts.BlueprintFeatureArgs {
		ref, key, value, err := parseBlueprintFeatureSpec(spec)
		if err != nil {
			return err
		}
		if opts.BlueprintFeatures[ref] == nil {
			opts.BlueprintFeatures[ref] = make(map[string]bool)
		}
		opts.BlueprintFeatures[ref][key] = value
	}

	for _, spec := range opts.BlueprintEntrypointArgs {
		ref, entrypoint, err := parseBlueprintEntrypointSpec(spec)
		if err != nil {
			return err
		}
		opts.BlueprintEntrypoints[ref] = entrypoint
	}

	return nil
}

func parseBlueprintParamSpec(spec string) (ref, key string, value interface{}, err error) {
	ref, rest, err := splitBlueprintSpec(spec)
	if err != nil {
		return "", "", nil, err
	}
	var valueStr string
	key, valueStr, err = splitKeyValue(rest)
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid blueprint param %q: %w", spec, err)
	}
	var parsed interface{}
	if err = yaml.Unmarshal([]byte(valueStr), &parsed); err != nil {
		return "", "", nil, fmt.Errorf("invalid blueprint param value %q: %w", valueStr, err)
	}
	return ref, key, parsed, nil
}

func parseBlueprintFeatureSpec(spec string) (ref, key string, enabled bool, err error) {
	ref, rest, err := splitBlueprintSpec(spec)
	if err != nil {
		return "", "", false, err
	}
	key, value, err := splitKeyValue(rest)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid blueprint feature %q: %w", spec, err)
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid blueprint feature value %q: %w", value, err)
	}
	return ref, key, parsed, nil
}

func parseBlueprintEntrypointSpec(spec string) (ref, entrypoint string, err error) {
	ref, rest, err := splitBlueprintSpec(spec)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(rest) == "" {
		return "", "", fmt.Errorf("invalid blueprint entrypoint %q: missing entrypoint", spec)
	}
	return ref, strings.TrimSpace(rest), nil
}

func splitBlueprintSpec(spec string) (ref, rest string, err error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid blueprint override %q: expected blueprint:setting", spec)
	}
	ref = strings.TrimSpace(parts[0])
	if ref == "" {
		return "", "", fmt.Errorf("invalid blueprint override %q: empty blueprint reference", spec)
	}
	return normalizeBlueprintKey(ref), parts[1], nil
}

func splitKeyValue(value string) (key, val string, err error) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected key=value")
	}
	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}
	return key, strings.TrimSpace(parts[1]), nil
}

func normalizeBlueprintKey(ref string) string {
	name, _ := blueprint.ParseBlueprintReference(strings.TrimSpace(ref))
	if name == "" {
		return strings.TrimSpace(ref)
	}
	return name
}
