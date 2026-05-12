package main

import (
	"reflect"
	"testing"
)

func TestSelectEntries(t *testing.T) {
	entries := []backlogEntry{
		{Title: "a", Version: "v1.1"},
		{Title: "b", Version: "v1.2"},
		{Title: "c", Version: "v1.1"},
		{Title: "d", Narrowing: true},
		{Title: "e", Narrowing: true},
		{Title: "f", Version: "v1.1", Narrowing: true}, // narrowing targeted at v1.1
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
		{"single version pulls in narrowing targeted there", []string{"v1.1"}, []string{"a", "c", "f"}},
		{"multiple versions", []string{"v1.1", "v1.2"}, []string{"a", "b", "c", "f"}},
		{"narrowings tag matches every narrowing", []string{narrowingsVersionTag}, []string{"d", "e", "f"}},
		{"version plus narrowings tag", []string{"v1.2", narrowingsVersionTag}, []string{"b", "d", "e", "f"}},
		{"unknown version", []string{"v9.9"}, nil},
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
		{"v1.1", []string{"v1.1"}},
		{"v1.1,v1.2", []string{"v1.1", "v1.2"}},
		{" v1.1 , , v1.2 ,", []string{"v1.1", "v1.2"}},
	}
	for _, tt := range tests {
		if got := splitCSV(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
