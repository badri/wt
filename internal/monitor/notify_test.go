package monitor

import "testing"

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "double quotes",
			input:    `Session "test" ready`,
			expected: `Session \"test\" ready`,
		},
		{
			name:     "backslashes",
			input:    `Path: C:\Users\test`,
			expected: `Path: C:\\Users\\test`,
		},
		{
			name:     "mixed quotes and backslashes",
			input:    `Say "hello\" to world`,
			expected: `Say \"hello\\\" to world`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeAppleScript(tt.input)
			if result != tt.expected {
				t.Errorf("escapeAppleScript(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
