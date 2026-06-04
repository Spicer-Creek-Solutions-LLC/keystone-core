// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestMajorOf(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"v1.2.3", 1, true},
		{"v2.0.0-rc.1", 2, true},
		{"v3.1.0+incompatible", 3, true},
		{"v0.27.0", 0, true},
		{"v10.4.1", 10, true},
		{"v0.0.0-20220722155257-8c9f86f7a55f", 0, true}, // pseudo-version: clean v0 major
		{"garbage", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := majorOf(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("majorOf(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSameMajor(t *testing.T) {
	cases := []struct {
		cur, upd string
		want     bool
	}{
		{"v1.2.3", "v1.5.0", true},   // minor bump
		{"v1.2.3", "v2.0.1", false},  // major bump → review
		{"v2.14.0", "v2.14.2", true}, // patch bump (the nats case)
		{"v0.1.0", "v0.2.0", true},   // pre-1.0 same (v0) major
		{"v1.0.0", "weird", false},   // unparseable update → surface as major
		{"weird", "v1.0.0", false},   // unparseable current → surface as major
	}
	for _, c := range cases {
		if got := sameMajor(c.cur, c.upd); got != c.want {
			t.Errorf("sameMajor(%q,%q) = %v, want %v", c.cur, c.upd, got, c.want)
		}
	}
}
