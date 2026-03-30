package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/docker/docker-agent/pkg/agent"
	"github.com/docker/docker-agent/pkg/chat"
	"github.com/docker/docker-agent/pkg/compaction"
	"github.com/docker/docker-agent/pkg/session"
)

func TestExtractMessagesToCompact(t *testing.T) {
	newMsg := func(role chat.MessageRole, content string) session.Item {
		return session.NewMessageItem(&session.Message{
			Message: chat.Message{Role: role, Content: content},
		})
	}

	tests := []struct {
		name                     string
		messages                 []session.Item
		contextLimit             int64
		additionalPrompt         string
		wantConversationMsgCount int
	}{
		{
			name:                     "empty session returns system and user prompt only",
			messages:                 nil,
			contextLimit:             100_000,
			wantConversationMsgCount: 0,
		},
		{
			name: "system messages are filtered out",
			messages: []session.Item{
				newMsg(chat.MessageRoleSystem, "system instruction"),
				newMsg(chat.MessageRoleUser, "hello"),
				newMsg(chat.MessageRoleAssistant, "hi"),
			},
			contextLimit:             100_000,
			wantConversationMsgCount: 2,
		},
		{
			name: "messages fit within context limit",
			messages: []session.Item{
				newMsg(chat.MessageRoleUser, "msg1"),
				newMsg(chat.MessageRoleAssistant, "msg2"),
				newMsg(chat.MessageRoleUser, "msg3"),
				newMsg(chat.MessageRoleAssistant, "msg4"),
			},
			contextLimit:             100_000,
			wantConversationMsgCount: 4,
		},
		{
			name: "truncation when context limit is very small",
			messages: []session.Item{
				newMsg(chat.MessageRoleUser, "first message with lots of content that takes tokens"),
				newMsg(chat.MessageRoleAssistant, "first response with lots of content that takes tokens"),
				newMsg(chat.MessageRoleUser, "second message"),
				newMsg(chat.MessageRoleAssistant, "second response"),
			},
			// Set context limit so small that after subtracting maxSummaryTokens + prompt overhead,
			// not all messages fit.
			contextLimit:             maxSummaryTokens + 50,
			wantConversationMsgCount: 0,
		},
		{
			name: "additional prompt is appended",
			messages: []session.Item{
				newMsg(chat.MessageRoleUser, "hello"),
			},
			contextLimit:             100_000,
			additionalPrompt:         "focus on code quality",
			wantConversationMsgCount: 1,
		},
		{
			name: "cost and cache control are cleared",
			messages: []session.Item{
				session.NewMessageItem(&session.Message{
					Message: chat.Message{
						Role:         chat.MessageRoleUser,
						Content:      "hello",
						Cost:         1.5,
						CacheControl: true,
					},
				}),
			},
			contextLimit:             100_000,
			wantConversationMsgCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := session.New(session.WithMessages(tt.messages))

			a := agent.New("test", "test prompt")
			result := extractMessagesToCompact(sess, a, tt.contextLimit, tt.additionalPrompt)

			assert.GreaterOrEqual(t, len(result), tt.wantConversationMsgCount+2)
			assert.Equal(t, chat.MessageRoleSystem, result[0].Role)
			assert.Equal(t, compaction.SystemPrompt, result[0].Content)

			last := result[len(result)-1]
			assert.Equal(t, chat.MessageRoleUser, last.Role)
			expectedPrompt := compaction.UserPrompt
			if tt.additionalPrompt != "" {
				expectedPrompt += "\n\n" + tt.additionalPrompt
			}
			assert.Equal(t, expectedPrompt, last.Content)

			// Conversation messages are all except first (system) and last (user prompt)
			assert.Equal(t, tt.wantConversationMsgCount, len(result)-2)

			// Verify cost and cache control are cleared on conversation messages
			for i := 1; i < len(result)-1; i++ {
				assert.Zero(t, result[i].Cost)
				assert.False(t, result[i].CacheControl)
			}
		})
	}
}
