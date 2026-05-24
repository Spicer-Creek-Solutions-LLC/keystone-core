// SPDX-License-Identifier: Apache-2.0

package main

import (
	_ "embed"
	"fmt"
	"io"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

//go:embed config/milestones.yaml
var milestonesYAML []byte

type milestoneSpec struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	DueOn       string `yaml:"due_on"`
}

func loadMilestoneSpecs() ([]milestoneSpec, error) {
	var specs []milestoneSpec
	if err := yaml.Unmarshal(milestonesYAML, &specs); err != nil {
		return nil, fmt.Errorf("parse config/milestones.yaml: %w", err)
	}
	for i, s := range specs {
		if strings.TrimSpace(s.Title) == "" {
			return nil, fmt.Errorf("config/milestones.yaml entry %d: title is required", i)
		}
	}
	return specs, nil
}

func milestoneDiffers(cur milestone, s milestoneSpec) bool {
	return strings.TrimSpace(cur.Description) != strings.TrimSpace(s.Description) ||
		(s.DueOn != "" && cur.DueOn != s.DueOn)
}

func syncMilestones(c *client, apply bool, out io.Writer) error {
	specs, err := loadMilestoneSpecs()
	if err != nil {
		return err
	}
	existing, err := c.listMilestones()
	if err != nil {
		return err
	}
	byTitle := make(map[string]milestone, len(existing))
	for _, m := range existing {
		byTitle[m.Title] = m
	}
	var created, updated, unchanged int
	for _, s := range specs {
		desc := strings.TrimSpace(s.Description)
		cur, ok := byTitle[s.Title]
		switch {
		case !ok:
			fmt.Fprintf(out, "+ create milestone %q\n", s.Title)
			created++
			if apply {
				if _, err := c.createMilestone(milestonePayload{Title: s.Title, Description: desc, DueOn: s.DueOn}); err != nil {
					return err
				}
			}
		case milestoneDiffers(cur, s):
			fmt.Fprintf(out, "~ update milestone %q\n", s.Title)
			updated++
			if apply {
				if _, err := c.editMilestone(cur.ID, milestonePayload{Title: s.Title, Description: desc, DueOn: s.DueOn}); err != nil {
					return err
				}
			}
		default:
			unchanged++
		}
	}
	fmt.Fprintf(out, "milestones: %d created, %d updated, %d unchanged\n", created, updated, unchanged)
	return nil
}
