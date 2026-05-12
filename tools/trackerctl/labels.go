package main

import (
	_ "embed"
	"fmt"
	"io"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

//go:embed config/labels.yaml
var labelsYAML []byte

type labelSpec struct {
	Name        string `yaml:"name"`
	Color       string `yaml:"color"`
	Description string `yaml:"description"`
	Exclusive   bool   `yaml:"exclusive"`
}

func loadLabelSpecs() ([]labelSpec, error) {
	var specs []labelSpec
	if err := yaml.Unmarshal(labelsYAML, &specs); err != nil {
		return nil, fmt.Errorf("parse config/labels.yaml: %w", err)
	}
	for i, s := range specs {
		if s.Name == "" || s.Color == "" {
			return nil, fmt.Errorf("config/labels.yaml entry %d: name and color are required", i)
		}
	}
	return specs, nil
}

func normColor(c string) string { return strings.ToLower(strings.TrimPrefix(c, "#")) }

func labelDiffers(cur label, s labelSpec) bool {
	return normColor(cur.Color) != normColor(s.Color) ||
		cur.Description != s.Description ||
		cur.Exclusive != s.Exclusive
}

func (s labelSpec) payload() labelPayload {
	return labelPayload{Name: s.Name, Color: "#" + normColor(s.Color), Description: s.Description, Exclusive: s.Exclusive}
}

func syncLabels(c *client, apply bool, out io.Writer) error {
	specs, err := loadLabelSpecs()
	if err != nil {
		return err
	}
	existing, err := c.listLabels()
	if err != nil {
		return err
	}
	byName := make(map[string]label, len(existing))
	for _, l := range existing {
		byName[l.Name] = l
	}
	wanted := make(map[string]bool, len(specs))
	var created, updated, unchanged int
	for _, s := range specs {
		wanted[s.Name] = true
		cur, ok := byName[s.Name]
		switch {
		case !ok:
			fmt.Fprintf(out, "+ create label %q (%s)\n", s.Name, s.Color)
			created++
			if apply {
				if _, err := c.createLabel(s.payload()); err != nil {
					return err
				}
			}
		case labelDiffers(cur, s):
			fmt.Fprintf(out, "~ update label %q\n", s.Name)
			updated++
			if apply {
				if _, err := c.editLabel(cur.ID, s.payload()); err != nil {
					return err
				}
			}
		default:
			unchanged++
		}
	}
	for _, l := range existing {
		if !wanted[l.Name] {
			fmt.Fprintf(out, "! extra label not in config (left untouched): %q\n", l.Name)
		}
	}
	fmt.Fprintf(out, "labels: %d created, %d updated, %d unchanged\n", created, updated, unchanged)
	return nil
}
