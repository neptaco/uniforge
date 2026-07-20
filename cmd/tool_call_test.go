package cmd

import (
	"strings"
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
)

func TestToolCallFailureError(t *testing.T) {
	tests := []struct {
		name            string
		result          *bridge.ClientToolCallResult
		wantExact       string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "uses result message",
			result: &bridge.ClientToolCallResult{
				Result: map[string]any{
					"message":    "3 test(s) failed",
					"fail_count": 3,
				},
			},
			wantExact:       "tool run-tests failed: 3 test(s) failed",
			wantNotContains: []string{"hint:", "unknown error"},
		},
		{
			name: "prefers error over result message",
			result: &bridge.ClientToolCallResult{
				Result: map[string]any{"message": "x"},
				Error:  "boom",
			},
			wantExact:       "tool run-tests failed: boom",
			wantNotContains: []string{"hint:"},
		},
		{
			name:            "uses unknown error and hint without result",
			result:          &bridge.ClientToolCallResult{},
			wantContains:    []string{"unknown error", "hint:"},
			wantNotContains: nil,
		},
		{
			name: "uses error and hint without result",
			result: &bridge.ClientToolCallResult{
				Error: "No tests matched",
			},
			wantContains:    []string{"No tests matched", "hint:"},
			wantNotContains: nil,
		},
		{
			name: "uses unknown error without hint for result without message",
			result: &bridge.ClientToolCallResult{
				Result: map[string]any{"fail_count": 3},
			},
			wantContains:    []string{"unknown error"},
			wantNotContains: []string{"hint:"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := toolCallFailureError("run-tests", test.result)
			if err == nil {
				t.Fatal("toolCallFailureError() error = nil, want non-nil")
			}

			got := err.Error()
			if test.wantExact != "" && got != test.wantExact {
				t.Fatalf("toolCallFailureError() = %q, want %q", got, test.wantExact)
			}
			for _, want := range test.wantContains {
				if !strings.Contains(got, want) {
					t.Fatalf("toolCallFailureError() = %q, want it to contain %q", got, want)
				}
			}
			for _, unwanted := range test.wantNotContains {
				if strings.Contains(got, unwanted) {
					t.Fatalf("toolCallFailureError() = %q, do not want it to contain %q", got, unwanted)
				}
			}
		})
	}
}
