package main

import (
	"reflect"
	"testing"
)

func TestSelectEntries(t *testing.T) {
	entries := []backlogEntry{
		{Title: "a", Version: "gate-v0.5"},
		{Title: "b", Version: "gate-v1.0"},
		{Title: "c", Version: "gate-v0.5"},
		{Title: "d", Version: "v0.x"},
		{Title: "e", Version: "v1.x"},
		{Title: "f", Version: "v2.x+"},
	}
	titles := func(es []backlogEntry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Title)
		}
		return out
	}

	tests := []struct {
		name     string
		versions []string
		want     []string
	}{
		{"empty means all", nil, []string{"a", "b", "c", "d", "e", "f"}},
		{"single bucket", []string{"gate-v0.5"}, []string{"a", "c"}},
		{"multiple buckets", []string{"gate-v0.5", "gate-v1.0"}, []string{"a", "b", "c"}},
		{"post-v1.0 buckets", []string{"v1.x", "v2.x+"}, []string{"e", "f"}},
		{"unknown bucket", []string{"v9.9"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titles(selectEntries(entries, tt.versions))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selectEntries(%v) = %v, want %v", tt.versions, got, tt.want)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"gate-v0.5", []string{"gate-v0.5"}},
		{"gate-v0.5,gate-v1.0", []string{"gate-v0.5", "gate-v1.0"}},
		{" gate-v0.5 , , gate-v1.0 ,", []string{"gate-v0.5", "gate-v1.0"}},
	}
	for _, tt := range tests {
		if got := splitCSV(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
