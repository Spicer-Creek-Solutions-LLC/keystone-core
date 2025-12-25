package targeting

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "simple key:value",
			expression: "os:linux",
			wantErr:    false,
		},
		{
			name:       "glob pattern",
			expression: "name:web-*",
			wantErr:    false,
		},
		{
			name:       "and operator",
			expression: "os:linux and role:web",
			wantErr:    false,
		},
		{
			name:       "or operator",
			expression: "os:linux or os:darwin",
			wantErr:    false,
		},
		{
			name:       "not operator",
			expression: "not os:windows",
			wantErr:    false,
		},
		{
			name:       "complex expression with grouping",
			expression: "(os:linux or os:darwin) and not role:db",
			wantErr:    false,
		},
		{
			name:       "multiple conditions",
			expression: "os:linux and role:web and datacenter:us-west",
			wantErr:    false,
		},
		{
			name:        "empty expression",
			expression:  "",
			wantErr:     true,
			errContains: "empty target expression",
		},
		{
			name:       "complex glob pattern",
			expression: "hostname:*.example.com and name:prod-*",
			wantErr:    false,
		},
		{
			name:       "nested parentheses",
			expression: "((os:linux and role:web) or (os:darwin and role:api)) and not env:test",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := Parse(tt.expression)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Parse() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Parse() unexpected error: %v", err)
				return
			}

			if expr == nil {
				t.Errorf("Parse() returned nil expression")
				return
			}

			if expr.String() != tt.expression {
				t.Errorf("Parse() String() = %q, want %q", expr.String(), tt.expression)
			}
		})
	}
}

func TestTargetExpression_Matches(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		metadata   map[string]string
		want       bool
		wantErr    bool
	}{
		{
			name:       "exact match",
			expression: "os:linux",
			metadata: map[string]string{
				"os": "linux",
			},
			want: true,
		},
		{
			name:       "exact no match",
			expression: "os:linux",
			metadata: map[string]string{
				"os": "darwin",
			},
			want: false,
		},
		{
			name:       "missing field",
			expression: "os:linux",
			metadata: map[string]string{
				"role": "web",
			},
			want: false,
		},
		{
			name:       "glob match",
			expression: "name:web-*",
			metadata: map[string]string{
				"name": "web-server-01",
			},
			want: true,
		},
		{
			name:       "glob no match",
			expression: "name:web-*",
			metadata: map[string]string{
				"name": "db-server-01",
			},
			want: false,
		},
		{
			name:       "hostname glob",
			expression: "hostname:*.example.com",
			metadata: map[string]string{
				"hostname": "server1.example.com",
			},
			want: true,
		},
		{
			name:       "and operator both true",
			expression: "os:linux and role:web",
			metadata: map[string]string{
				"os":   "linux",
				"role": "web",
			},
			want: true,
		},
		{
			name:       "and operator one false",
			expression: "os:linux and role:web",
			metadata: map[string]string{
				"os":   "linux",
				"role": "db",
			},
			want: false,
		},
		{
			name:       "or operator one true",
			expression: "os:linux or os:darwin",
			metadata: map[string]string{
				"os": "linux",
			},
			want: true,
		},
		{
			name:       "or operator both false",
			expression: "os:linux or os:darwin",
			metadata: map[string]string{
				"os": "windows",
			},
			want: false,
		},
		{
			name:       "not operator",
			expression: "not os:windows",
			metadata: map[string]string{
				"os": "linux",
			},
			want: true,
		},
		{
			name:       "not operator negated",
			expression: "not os:windows",
			metadata: map[string]string{
				"os": "windows",
			},
			want: false,
		},
		{
			name:       "complex expression true",
			expression: "(os:linux or os:darwin) and not role:db",
			metadata: map[string]string{
				"os":   "linux",
				"role": "web",
			},
			want: true,
		},
		{
			name:       "complex expression false by not",
			expression: "(os:linux or os:darwin) and not role:db",
			metadata: map[string]string{
				"os":   "linux",
				"role": "db",
			},
			want: false,
		},
		{
			name:       "complex expression false by first clause",
			expression: "(os:linux or os:darwin) and not role:db",
			metadata: map[string]string{
				"os":   "windows",
				"role": "web",
			},
			want: false,
		},
		{
			name:       "multiple and conditions",
			expression: "os:linux and role:web and datacenter:us-west",
			metadata: map[string]string{
				"os":         "linux",
				"role":       "web",
				"datacenter": "us-west",
			},
			want: true,
		},
		{
			name:       "multiple and conditions one missing",
			expression: "os:linux and role:web and datacenter:us-west",
			metadata: map[string]string{
				"os":         "linux",
				"role":       "web",
				"datacenter": "us-east",
			},
			want: false,
		},
		{
			name:       "glob with multiple wildcards",
			expression: "name:web-*-prod",
			metadata: map[string]string{
				"name": "web-server-01-prod",
			},
			want: true,
		},
		{
			name:       "nested parentheses",
			expression: "((os:linux and role:web) or (os:darwin and role:api)) and not env:test",
			metadata: map[string]string{
				"os":   "linux",
				"role": "web",
				"env":  "prod",
			},
			want: true,
		},
		{
			name:       "nested parentheses second branch",
			expression: "((os:linux and role:web) or (os:darwin and role:api)) and not env:test",
			metadata: map[string]string{
				"os":   "darwin",
				"role": "api",
				"env":  "prod",
			},
			want: true,
		},
		{
			name:       "nested parentheses env test",
			expression: "((os:linux and role:web) or (os:darwin and role:api)) and not env:test",
			metadata: map[string]string{
				"os":   "linux",
				"role": "web",
				"env":  "test",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := Parse(tt.expression)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			got, err := expr.Matches(tt.metadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("Matches() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("Matches() = %v, want %v (metadata: %v)", got, tt.want, tt.metadata)
			}
		})
	}
}

func TestMatchValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		pattern string
		want    bool
	}{
		{
			name:    "exact match",
			value:   "linux",
			pattern: "linux",
			want:    true,
		},
		{
			name:    "exact no match",
			value:   "linux",
			pattern: "windows",
			want:    false,
		},
		{
			name:    "glob star prefix",
			value:   "web-server-01",
			pattern: "web-*",
			want:    true,
		},
		{
			name:    "glob star suffix",
			value:   "server-01-web",
			pattern: "*-web",
			want:    true,
		},
		{
			name:    "glob star both",
			value:   "web-server-01-prod",
			pattern: "*-server-*",
			want:    true,
		},
		{
			name:    "glob question mark",
			value:   "web-1",
			pattern: "web-?",
			want:    true,
		},
		{
			name:    "glob complex pattern",
			value:   "server1.example.com",
			pattern: "*.example.com",
			want:    true,
		},
		{
			name:    "glob no match",
			value:   "database-server",
			pattern: "web-*",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchValue(tt.value, tt.pattern)
			if got != tt.want {
				t.Errorf("matchValue(%q, %q) = %v, want %v", tt.value, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestConvertToExprSyntax(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "simple key:value",
			input:  "os:linux",
			output: "match('os', 'linux')",
		},
		{
			name:   "and operator",
			input:  "os:linux and role:web",
			output: "match('os', 'linux') && match('role', 'web')",
		},
		{
			name:   "or operator",
			input:  "os:linux or os:darwin",
			output: "match('os', 'linux') || match('os', 'darwin')",
		},
		{
			name:   "not operator",
			input:  "not os:windows",
			output: "!match('os', 'windows')",
		},
		{
			name:   "parentheses",
			input:  "(os:linux or os:darwin) and role:web",
			output: "(match('os', 'linux') || match('os', 'darwin')) && match('role', 'web')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToExprSyntax(tt.input)
			if got != tt.output {
				t.Errorf("convertToExprSyntax(%q) = %q, want %q", tt.input, got, tt.output)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output []string
	}{
		{
			name:   "simple expression",
			input:  "os:linux",
			output: []string{"os:linux"},
		},
		{
			name:   "with and",
			input:  "os:linux and role:web",
			output: []string{"os:linux", "and", "role:web"},
		},
		{
			name:   "with parentheses",
			input:  "(os:linux or os:darwin)",
			output: []string{"(", "os:linux", "or", "os:darwin", ")"},
		},
		{
			name:   "complex expression",
			input:  "(os:linux and role:web) or name:special",
			output: []string{"(", "os:linux", "and", "role:web", ")", "or", "name:special"},
		},
		{
			name:   "not operator",
			input:  "not os:windows",
			output: []string{"not", "os:windows"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.output) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.output)
				return
			}
			for i := range got {
				if got[i] != tt.output[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.output[i])
				}
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
