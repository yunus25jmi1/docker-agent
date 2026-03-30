// Package hooks provides lifecycle hooks for agent tool execution.
// Hooks allow users to run shell commands or prompts at various points
// during the agent's execution lifecycle, providing deterministic control
// over agent behavior.
package hooks

import (
	"encoding/json"
	"time"
)

// EventType represents the type of hook event
type EventType string

const (
	// EventPreToolUse is triggered before a tool call executes.
	// Can allow/deny/modify tool calls; can block with feedback.
	EventPreToolUse EventType = "pre_tool_use"

	// EventPostToolUse is triggered after a tool completes successfully.
	// Can provide validation, feedback, or additional processing.
	EventPostToolUse EventType = "post_tool_use"

	// EventSessionStart is triggered when a session begins or resumes.
	// Can load context, setup environment, install dependencies.
	EventSessionStart EventType = "session_start"

	// EventSessionEnd is triggered when a session terminates.
	// Can perform cleanup, logging, persist session state.
	EventSessionEnd EventType = "session_end"

	// EventOnUserInput is triggered when the agent needs input from the user.
	// Can log, notify, or perform actions before user interaction.
	EventOnUserInput EventType = "on_user_input"

	// EventStop is triggered when the model finishes its response and is about
	// to hand control back to the user. Can perform post-response validation,
	// logging, or cleanup.
	EventStop EventType = "stop"

	// EventNotification is triggered when the agent emits a notification to the user,
	// such as errors or warnings. Can send external notifications or log events.
	EventNotification EventType = "notification"
)

// HookType represents the type of hook action
type HookType string

const (
	// HookTypeCommand executes a shell command
	HookTypeCommand HookType = "command"
)

// Hook represents a single hook configuration
type Hook struct {
	// Type specifies whether this is a command or prompt hook
	Type HookType `json:"type" yaml:"type"`

	// Command is the shell command to execute (for command hooks)
	Command string `json:"command,omitempty" yaml:"command,omitempty"`

	// Timeout is the execution timeout in seconds (default: 60)
	Timeout int `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// GetTimeout returns the timeout duration, defaulting to 60 seconds
func (h *Hook) GetTimeout() time.Duration {
	if h.Timeout <= 0 {
		return 60 * time.Second
	}
	return time.Duration(h.Timeout) * time.Second
}

// MatcherConfig represents a hook matcher with its hooks
type MatcherConfig struct {
	// Matcher is a regex pattern to match tool names (e.g., "shell|edit_file")
	// Use "*" to match all tools
	Matcher string `json:"matcher,omitempty" yaml:"matcher,omitempty"`

	// Hooks are the hooks to execute when the matcher matches
	Hooks []Hook `json:"hooks" yaml:"hooks"`
}

// Config represents the hooks configuration for an agent
type Config struct {
	// PreToolUse hooks run before tool execution
	PreToolUse []MatcherConfig `json:"pre_tool_use,omitempty" yaml:"pre_tool_use,omitempty"`

	// PostToolUse hooks run after tool execution
	PostToolUse []MatcherConfig `json:"post_tool_use,omitempty" yaml:"post_tool_use,omitempty"`

	// SessionStart hooks run when a session begins
	SessionStart []Hook `json:"session_start,omitempty" yaml:"session_start,omitempty"`

	// SessionEnd hooks run when a session ends
	SessionEnd []Hook `json:"session_end,omitempty" yaml:"session_end,omitempty"`

	// OnUserInput hooks run when the agent needs user input
	OnUserInput []Hook `json:"on_user_input,omitempty" yaml:"on_user_input,omitempty"`

	// Stop hooks run when the model finishes responding
	Stop []Hook `json:"stop,omitempty" yaml:"stop,omitempty"`

	// Notification hooks run when the agent sends a notification (error, warning) to the user
	Notification []Hook `json:"notification,omitempty" yaml:"notification,omitempty"`
}

// IsEmpty returns true if no hooks are configured
func (c *Config) IsEmpty() bool {
	return len(c.PreToolUse) == 0 &&
		len(c.PostToolUse) == 0 &&
		len(c.SessionStart) == 0 &&
		len(c.SessionEnd) == 0 &&
		len(c.OnUserInput) == 0 &&
		len(c.Stop) == 0 &&
		len(c.Notification) == 0
}

// Input represents the JSON input passed to hooks via stdin
type Input struct {
	// Common fields for all hooks
	SessionID     string    `json:"session_id"`
	Cwd           string    `json:"cwd"`
	HookEventName EventType `json:"hook_event_name"`

	// Tool-related fields (for PreToolUse and PostToolUse)
	ToolName  string         `json:"tool_name,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	ToolInput map[string]any `json:"tool_input,omitempty"`

	// PostToolUse specific
	ToolResponse any `json:"tool_response,omitempty"`

	// SessionStart specific
	Source string `json:"source,omitempty"` // "startup", "resume", "clear", "compact"

	// SessionEnd specific
	Reason string `json:"reason,omitempty"` // "clear", "logout", "prompt_input_exit", "other"

	// Stop specific
	StopResponse string `json:"stop_response,omitempty"` // The model's final response content

	// Notification specific
	NotificationLevel   string `json:"notification_level,omitempty"`   // "error" or "warning"
	NotificationMessage string `json:"notification_message,omitempty"` // The notification content
}

// ToJSON serializes the input to JSON
func (i *Input) ToJSON() ([]byte, error) {
	return json.Marshal(i)
}

// Decision represents a permission decision from a hook
type Decision string

const (
	// DecisionAllow allows the operation to proceed
	DecisionAllow Decision = "allow"

	// DecisionDeny blocks the operation
	DecisionDeny Decision = "deny"

	// DecisionAsk prompts the user for confirmation (PreToolUse only)
	DecisionAsk Decision = "ask"
)

// Output represents the JSON output from a hook
type Output struct {
	// Continue indicates whether to continue execution (default: true)
	Continue *bool `json:"continue,omitempty"`

	// StopReason is the message to show when continue=false
	StopReason string `json:"stop_reason,omitempty"`

	// SuppressOutput hides stdout from transcript
	SuppressOutput bool `json:"suppress_output,omitempty"`

	// SystemMessage is a warning to show the user
	SystemMessage string `json:"system_message,omitempty"`

	// Decision is for blocking operations
	Decision string `json:"decision,omitempty"`

	// Reason is the message explaining the decision
	Reason string `json:"reason,omitempty"`

	// HookSpecificOutput contains event-specific fields
	HookSpecificOutput *HookSpecificOutput `json:"hook_specific_output,omitempty"`
}

// ShouldContinue returns whether execution should continue
func (o *Output) ShouldContinue() bool {
	if o.Continue == nil {
		return true
	}
	return *o.Continue
}

// IsBlocked returns true if the decision is to block
func (o *Output) IsBlocked() bool {
	return o.Decision == "block"
}

// HookSpecificOutput contains event-specific output fields
type HookSpecificOutput struct {
	// HookEventName identifies which event this output is for
	HookEventName EventType `json:"hook_event_name,omitempty"`

	// PreToolUse fields
	PermissionDecision       Decision       `json:"permission_decision,omitempty"`
	PermissionDecisionReason string         `json:"permission_decision_reason,omitempty"`
	UpdatedInput             map[string]any `json:"updated_input,omitempty"`

	// PostToolUse/SessionStart fields
	AdditionalContext string `json:"additional_context,omitempty"`
}

// Result represents the result of executing hooks
type Result struct {
	// Allowed indicates if the operation should proceed
	Allowed bool

	// Message is feedback to include in the response
	Message string

	// ModifiedInput contains any modifications to tool input (PreToolUse only)
	ModifiedInput map[string]any

	// AdditionalContext is context to add (PostToolUse/SessionStart)
	AdditionalContext string

	// SystemMessage is a warning to show the user
	SystemMessage string

	// ExitCode is the exit code from the hook command (0 = success, 2 = blocking error)
	ExitCode int

	// Stderr contains any error output from the hook
	Stderr string
}
