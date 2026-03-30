package runtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/docker/docker-agent/pkg/agent"
	"github.com/docker/docker-agent/pkg/chat"
	"github.com/docker/docker-agent/pkg/compaction"
	"github.com/docker/docker-agent/pkg/model/provider"
	"github.com/docker/docker-agent/pkg/model/provider/options"
	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/team"
)

const maxSummaryTokens = 16_000

// doCompact runs compaction on a session and applies the result (events,
// persistence, token count updates). The agent is used to extract the
// conversation from the session and to obtain the model for summarization.
func (r *LocalRuntime) doCompact(ctx context.Context, sess *session.Session, a *agent.Agent, additionalPrompt string, events chan Event) {
	slog.Debug("Generating summary for session", "session_id", sess.ID)
	events <- SessionCompaction(sess.ID, "started", a.Name())
	defer func() {
		events <- SessionCompaction(sess.ID, "completed", a.Name())
	}()

	// Build a model just for compaction.
	summaryModel := provider.CloneWithOptions(ctx, a.Model(),
		options.WithStructuredOutput(nil),
		options.WithMaxTokens(maxSummaryTokens),
	)
	m, err := r.modelsStore.GetModel(ctx, summaryModel.ID())
	if err != nil {
		slog.Error("Failed to generate session summary", "error", errors.New("failed to get model definition"))
		events <- Error("Failed to get model definition")
		return
	}

	compactionAgent := agent.New("root", compaction.SystemPrompt, agent.WithModel(summaryModel))

	// Compute the messages to compact.
	messages := extractMessagesToCompact(sess, compactionAgent, int64(m.Limit.Context), additionalPrompt)

	// Run the compaction.
	compactionSession := session.New(
		session.WithTitle("Generating summary"),
		session.WithMessages(toItems(messages)),
	)

	t := team.New(team.WithAgents(compactionAgent))
	rt, err := New(t, WithSessionCompaction(false))
	if err != nil {
		slog.Error("Failed to generate session summary", "error", err)
		events <- Error(err.Error())
		return
	}
	if _, err = rt.Run(ctx, compactionSession); err != nil {
		slog.Error("Failed to generate session summary", "error", err)
		events <- Error(err.Error())
		return
	}

	summary := compactionSession.GetLastAssistantMessageContent()
	if summary == "" {
		return
	}

	// Update the session.
	sess.InputTokens = compactionSession.OutputTokens
	sess.OutputTokens = 0
	sess.Messages = append(sess.Messages, session.Item{
		Summary: summary,
		Cost:    compactionSession.TotalCost(),
	})
	_ = r.sessionStore.UpdateSession(ctx, sess)

	slog.Debug("Generated session summary", "session_id", sess.ID, "summary_length", len(summary))
	events <- SessionSummary(sess.ID, summary, a.Name())
}

func extractMessagesToCompact(sess *session.Session, compactionAgent *agent.Agent, contextLimit int64, additionalPrompt string) []chat.Message {
	// Add all the existing messages.
	var messages []chat.Message
	for _, msg := range sess.GetMessages(compactionAgent) {
		if msg.Role == chat.MessageRoleSystem {
			continue
		}

		msg.Cost = 0
		msg.CacheControl = false

		messages = append(messages, msg)
	}

	// Prepare the first (system) message.
	systemPromptMessage := chat.Message{
		Role:      chat.MessageRoleSystem,
		Content:   compaction.SystemPrompt,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	systemPromptMessageLen := compaction.EstimateMessageTokens(&systemPromptMessage)

	// Prepare the last (user) message.
	userPrompt := compaction.UserPrompt
	if additionalPrompt != "" {
		userPrompt += "\n\n" + additionalPrompt
	}
	userPromptMessage := chat.Message{
		Role:      chat.MessageRoleUser,
		Content:   userPrompt,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	userPromptMessageLen := compaction.EstimateMessageTokens(&userPromptMessage)

	// Truncate the messages so that they fit in the available context limit
	// (minus the expected max length of the summary).
	contextAvailable := max(0, contextLimit-maxSummaryTokens-systemPromptMessageLen-userPromptMessageLen)
	firstIndex := firstMessageToKeep(messages, contextAvailable)
	if firstIndex < len(messages) {
		messages = messages[firstIndex:]
	} else {
		messages = nil
	}

	// Prepend the first (system) message.
	messages = append([]chat.Message{systemPromptMessage}, messages...)

	// Append the last (user) message.
	messages = append(messages, userPromptMessage)

	return messages
}

func firstMessageToKeep(messages []chat.Message, contextLimit int64) int {
	var tokens int64

	lastValidMessageSeen := len(messages)

	for i := len(messages) - 1; i >= 0; i-- {
		tokens += compaction.EstimateMessageTokens(&messages[i])
		if tokens > contextLimit {
			return lastValidMessageSeen
		}

		role := messages[i].Role
		if role == chat.MessageRoleUser || role == chat.MessageRoleAssistant {
			lastValidMessageSeen = i
		}
	}

	return lastValidMessageSeen
}

func toItems(messages []chat.Message) []session.Item {
	var items []session.Item

	for _, message := range messages {
		items = append(items, session.Item{
			Message: &session.Message{
				Message: message,
			},
		})
	}

	return items
}
