package token

import (
	"testing"
	"time"
)

func TestJoinToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"future", time.Now().Add(time.Hour), false},
		{"past", time.Now().Add(-time.Hour), true},
		{"just expired", time.Now().Add(-time.Millisecond), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &JoinToken{ExpiresAt: tt.expiresAt}
			if got := tok.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinToken_HasUsesRemaining(t *testing.T) {
	tests := []struct {
		name      string
		maxUses   int
		usedCount int
		want      bool
	}{
		{"unlimited unused", 0, 0, true},
		{"unlimited used many", 0, 100, true},
		{"limited with remaining", 5, 3, true},
		{"limited at boundary", 5, 4, true},
		{"limited exhausted", 5, 5, false},
		{"limited over exhausted", 5, 6, false},
		{"single use unused", 1, 0, true},
		{"single use exhausted", 1, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &JoinToken{MaxUses: tt.maxUses, UsedCount: tt.usedCount}
			if got := tok.HasUsesRemaining(); got != tt.want {
				t.Errorf("HasUsesRemaining() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinToken_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		token   JoinToken
		want    bool
	}{
		{
			"valid token",
			JoinToken{ExpiresAt: time.Now().Add(time.Hour), MaxUses: 0, Revoked: false},
			true,
		},
		{
			"expired",
			JoinToken{ExpiresAt: time.Now().Add(-time.Hour), MaxUses: 0, Revoked: false},
			false,
		},
		{
			"revoked",
			JoinToken{ExpiresAt: time.Now().Add(time.Hour), MaxUses: 0, Revoked: true},
			false,
		},
		{
			"exhausted",
			JoinToken{ExpiresAt: time.Now().Add(time.Hour), MaxUses: 1, UsedCount: 1, Revoked: false},
			false,
		},
		{
			"revoked and expired",
			JoinToken{ExpiresAt: time.Now().Add(-time.Hour), MaxUses: 0, Revoked: true},
			false,
		},
		{
			"valid limited use",
			JoinToken{ExpiresAt: time.Now().Add(time.Hour), MaxUses: 5, UsedCount: 3, Revoked: false},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
