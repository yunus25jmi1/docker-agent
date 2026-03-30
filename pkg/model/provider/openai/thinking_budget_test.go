package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/config/latest"
)

func TestIsOpenAIReasoningModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		// O1 series models - should support reasoning
		{"o1-preview", "o1-preview", true},
		{"o1-mini", "o1-mini", true},
		{"O1-PREVIEW uppercase", "O1-PREVIEW", true},
		{"o1 with suffix", "o1-custom-version", true},

		// O3 series models - should support reasoning
		{"o3-mini", "o3-mini", true},
		{"o3-preview", "o3-preview", true},
		{"O3 uppercase", "O3", true},
		{"o3 with suffix", "o3-custom", true},

		// O4 series models - should support reasoning
		{"o4-preview", "o4-preview", true},
		{"o4 basic", "o4", true},
		{"O4 uppercase", "O4", true},

		// GPT-5 series models - should support reasoning
		{"gpt-5", "gpt-5", true},
		{"gpt-5-mini", "gpt-5-mini", true},
		{"gpt-5-turbo", "gpt-5-turbo", true},
		{"GPT-5 uppercase", "GPT-5", true},

		// GPT-5-chat variants are non-reasoning chat models
		{"gpt-5-chat-latest", "gpt-5-chat-latest", false},
		{"gpt-5-chat", "gpt-5-chat", false},
		{"GPT-5-CHAT uppercase", "GPT-5-CHAT-LATEST", false},

		// GPT-4 series models - should NOT support reasoning
		{"gpt-4", "gpt-4", false},
		{"gpt-4o", "gpt-4o", false},
		{"gpt-4-turbo", "gpt-4-turbo", false},
		{"gpt-4o-mini", "gpt-4o-mini", false},
		{"GPT-4O uppercase", "GPT-4O", false},

		// GPT-3.5 series models - should NOT support reasoning
		{"gpt-3.5-turbo", "gpt-3.5-turbo", false},
		{"gpt-3.5-turbo-16k", "gpt-3.5-turbo-16k", false},

		// O1-pro series models - should support reasoning
		{"o1-pro", "o1-pro", true},
		{"o1-pro-2025-03-19", "o1-pro-2025-03-19", true},
		{"O1-PRO uppercase", "O1-PRO", true},

		// Other models - should NOT support reasoning
		{"text-davinci-003", "text-davinci-003", false},
		{"gpt-3", "gpt-3", false},
		{"claude-3", "claude-3", false},
		{"gemini-pro", "gemini-pro", false},
		{"empty string", "", false},
		{"random model", "random-model-name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isOpenAIReasoningModel(tt.model)
			assert.Equal(t, tt.expected, result, "Model %s should return %v", tt.model, tt.expected)
		})
	}
}

func TestOpenAIReasoningEffort_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		budget         *latest.ThinkingBudget
		expectedEffort string
	}{
		{"minimal", &latest.ThinkingBudget{Effort: "minimal"}, "minimal"},
		{"low", &latest.ThinkingBudget{Effort: "low"}, "low"},
		{"medium", &latest.ThinkingBudget{Effort: "medium"}, "medium"},
		{"high", &latest.ThinkingBudget{Effort: "high"}, "high"},
		{"xhigh", &latest.ThinkingBudget{Effort: "xhigh"}, "xhigh"},
		{"xhigh uppercase", &latest.ThinkingBudget{Effort: "XHIGH"}, "xhigh"},
		{"uppercase", &latest.ThinkingBudget{Effort: "HIGH"}, "high"},
		{"whitespace", &latest.ThinkingBudget{Effort: "  medium  "}, "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			effort, err := openAIReasoningEffort(tt.budget)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedEffort, effort)
		})
	}
}

func TestOpenAIReasoningEffort_InvalidEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		budget        *latest.ThinkingBudget
		expectedError string
	}{
		{
			name:          "invalid effort level",
			budget:        &latest.ThinkingBudget{Effort: "invalid"},
			expectedError: "got effort: 'invalid', tokens: '0'",
		},
		{
			name:          "numeric string",
			budget:        &latest.ThinkingBudget{Effort: "2048"},
			expectedError: "got effort: '2048', tokens: '0'",
		},
		{
			name:          "tokens set but effort invalid",
			budget:        &latest.ThinkingBudget{Effort: "super-high", Tokens: 4096},
			expectedError: "got effort: 'super-high', tokens: '4096'",
		},
		{
			name:          "tokens only",
			budget:        &latest.ThinkingBudget{Tokens: 2048},
			expectedError: "got effort: '', tokens: '2048'",
		},
		{
			name:          "empty effort",
			budget:        &latest.ThinkingBudget{Effort: ""},
			expectedError: "got effort: '', tokens: '0'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			effort, err := openAIReasoningEffort(tt.budget)
			require.Error(t, err)
			assert.Empty(t, effort)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}
