package compaction

import (
	_ "embed"

	"github.com/docker/docker-agent/pkg/chat"
)

var (
	//go:embed prompts/compaction-system.txt
	SystemPrompt string

	//go:embed prompts/compaction-user.txt
	UserPrompt string
)

// contextThreshold is the fraction of the context window at which compaction
// is triggered. When the estimated token usage exceeds this fraction of the
// context limit, compaction is recommended.
const contextThreshold = 0.9

// ShouldCompact reports whether a session's context usage has crossed the
// compaction threshold. It returns true when the total token count
// (input + output + addedTokens) exceeds [contextThreshold] (90%) of
// contextLimit.
func ShouldCompact(inputTokens, outputTokens, addedTokens, contextLimit int64) bool {
	if contextLimit <= 0 {
		return false
	}
	return (inputTokens + outputTokens + addedTokens) > int64(float64(contextLimit)*contextThreshold)
}

// EstimateMessageTokens returns a rough token-count estimate for a single
// chat message based on its text length. This is intentionally conservative
// (overestimates) so that proactive compaction fires before we hit the limit.
//
// The estimate accounts for message content, multi-content text parts,
// reasoning content, tool call arguments, and a small per-message overhead
// for role/metadata tokens.
func EstimateMessageTokens(msg *chat.Message) int64 {
	// charsPerToken: average characters per token. 4 is a widely-used
	// heuristic for English; slightly overestimates for code/JSON (~3.5).
	const charsPerToken = 4

	// perMessageOverhead: role, ToolCallID, delimiters, etc.
	const perMessageOverhead = 5

	var chars int
	chars += len(msg.Content)
	for _, part := range msg.MultiContent {
		chars += len(part.Text)
	}
	chars += len(msg.ReasoningContent)
	for _, tc := range msg.ToolCalls {
		chars += len(tc.Function.Arguments)
		chars += len(tc.Function.Name)
	}

	if chars == 0 {
		return perMessageOverhead
	}
	return int64(chars/charsPerToken) + perMessageOverhead
}
