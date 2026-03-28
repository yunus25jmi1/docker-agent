// Package audit provides cryptographic audit trail functionality for agent actions.
// Each tool call, file operation, or external request gets a signed, timestamped record
// that's tamper-proof and independently verifiable.
package audit

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/docker-agent/pkg/paths"
	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/tools"
)

// ActionType represents the type of action being audited
type ActionType string

const (
	ActionTypeToolCall      ActionType = "tool_call"
	ActionTypeFileRead      ActionType = "file_read"
	ActionTypeFileWrite     ActionType = "file_write"
	ActionTypeFileDelete    ActionType = "file_delete"
	ActionTypeHTTPRequest   ActionType = "http_request"
	ActionTypeCommandExec   ActionType = "command_exec"
	ActionTypeSessionStart  ActionType = "session_start"
	ActionTypeSessionEnd    ActionType = "session_end"
	ActionTypeUserInput     ActionType = "user_input"
	ActionTypeModelResponse ActionType = "model_response"
	ActionTypeHandoff       ActionType = "handoff"
	ActionTypeError         ActionType = "error"
)

// AuditRecord represents a single auditable action with cryptographic signature
type AuditRecord struct {
	// ID is a unique identifier for this audit record
	ID string `json:"id"`

	// Timestamp is when the action occurred (RFC3339 format)
	Timestamp string `json:"timestamp"`

	// ActionType is the type of action being audited
	ActionType ActionType `json:"action_type"`

	// SessionID is the session where the action occurred
	SessionID string `json:"session_id"`

	// AgentName is the name of the agent that performed the action
	AgentName string `json:"agent_name"`

	// Action contains the details of the action (varies by action type)
	Action any `json:"action"`

	// Result contains the outcome of the action (if applicable)
	Result *ActionResult `json:"result,omitempty"`

	// PreviousHash is the SHA256 hash of the previous audit record (chain integrity)
	PreviousHash string `json:"previous_hash"`

	// Hash is the SHA256 hash of this record (excluding signature)
	Hash string `json:"hash"`

	// Signature is the Ed25519 signature of the hash
	Signature string `json:"signature"`

	// PublicKey is the base64-encoded public key for verification
	PublicKey string `json:"public_key"`
}

// ActionResult contains the result of an audited action
type ActionResult struct {
	Success  bool   `json:"success"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
	Metadata any    `json:"metadata,omitempty"`
}

// ToolCallAction contains details about a tool call action
type ToolCallAction struct {
	ToolName     string         `json:"tool_name"`
	ToolType     string         `json:"tool_type"`
	ToolCallID   string         `json:"tool_call_id"`
	Arguments    map[string]any `json:"arguments,omitempty"`
	RequiresAuth bool           `json:"requires_auth,omitempty"`
}

// FileAction contains details about a file operation
type FileAction struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// HTTPAction contains details about an HTTP request
type HTTPAction struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	StatusCode int               `json:"status_code,omitempty"`
	Response   string            `json:"response,omitempty"`
}

// CommandAction contains details about a command execution
type CommandAction struct {
	Command  string            `json:"command"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	WorkDir  string            `json:"work_dir,omitempty"`
	Output   string            `json:"output,omitempty"`
	ExitCode int               `json:"exit_code,omitempty"`
}

// UserInputAction contains details about user input
type UserInputAction struct {
	Input     string `json:"input"`
	Prompt    string `json:"prompt,omitempty"`
	Confirmed bool   `json:"confirmed,omitempty"`
}

// ModelResponseAction contains details about a model response
type ModelResponseAction struct {
	Model        string `json:"model"`
	TokensUsed   int    `json:"tokens_used,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// HandoffAction contains details about an agent handoff
type HandoffAction struct {
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	Reason    string `json:"reason,omitempty"`
}

// ErrorAction contains details about an error
type ErrorAction struct {
	Error       string `json:"error"`
	Code        string `json:"code,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
}

// Auditor handles audit trail recording and verification
type Auditor struct {
	mu          sync.Mutex
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
	records     []*AuditRecord
	storagePath string
	enabled     bool
	sessionHash map[string]string // Maps session ID to last hash for chaining
}

// Config holds auditor configuration
type Config struct {
	// Enabled enables audit trail recording
	Enabled bool
	// StoragePath is where to persist audit records (default: data dir)
	StoragePath string
	// KeyPath is where to store/load the signing key
	KeyPath string
}

// New creates a new Auditor with the given configuration
func New(cfg Config) (*Auditor, error) {
	auditor := &Auditor{
		enabled:     cfg.Enabled,
		storagePath: cfg.StoragePath,
		sessionHash: make(map[string]string),
		records:     make([]*AuditRecord, 0),
	}

	if !auditor.enabled {
		return auditor, nil
	}

	// Set default storage path
	if auditor.storagePath == "" {
		auditor.storagePath = filepath.Join(paths.GetDataDir(), "audit")
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(auditor.storagePath, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create audit storage directory: %w", err)
	}

	// Load or generate signing key
	keyPath := cfg.KeyPath
	if keyPath == "" {
		keyPath = filepath.Join(auditor.storagePath, "audit_key")
	}

	if err := auditor.loadOrGenerateKey(keyPath); err != nil {
		return nil, fmt.Errorf("failed to initialize signing key: %w", err)
	}

	return auditor, nil
}

// loadOrGenerateKey loads an existing key or generates a new one
func (a *Auditor) loadOrGenerateKey(keyPath string) error {
	// Try to load existing key
	data, err := os.ReadFile(keyPath)
	if err == nil {
		keyBytes, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return fmt.Errorf("failed to decode existing key: %w", err)
		}
		if len(keyBytes) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid key length: %d", len(keyBytes))
		}
		a.privateKey = ed25519.PrivateKey(keyBytes)
		a.publicKey = a.privateKey.Public().(ed25519.PublicKey)
		return nil
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing key: %w", err)
	}

	// Generate new key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	a.privateKey = priv
	a.publicKey = pub

	// Save key to disk
	keyData := base64.StdEncoding.EncodeToString(priv)
	if err := os.WriteFile(keyPath, []byte(keyData), 0o600); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}

	return nil
}

// Enabled returns true if auditing is enabled
func (a *Auditor) Enabled() bool {
	return a.enabled
}

// RecordToolCall records a tool call action
func (a *Auditor) RecordToolCall(ctx context.Context, sess *session.Session, agentName string, toolCall tools.ToolCall, result *tools.ToolCallResult, duration time.Duration) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	var args map[string]any
	if toolCall.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			args = map[string]any{"raw": toolCall.Function.Arguments}
		}
	}

	action := &ToolCallAction{
		ToolName:   toolCall.Function.Name,
		ToolType:   string(toolCall.Type),
		ToolCallID: toolCall.ID,
		Arguments:  args,
	}

	var actionResult *ActionResult
	if result != nil {
		actionResult = &ActionResult{
			Success:  !result.IsError,
			Output:   result.Output,
			Duration: duration.String(),
		}
		if result.IsError {
			actionResult.Error = result.Output
		}
	}

	return a.record(ctx, sess, agentName, ActionTypeToolCall, action, actionResult)
}

// RecordFileOperation records a file operation
func (a *Auditor) RecordFileOperation(ctx context.Context, sess *session.Session, agentName string, actionType ActionType, path string, content string, mode string) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	fileAction := &FileAction{
		Path:    path,
		Content: content,
		Mode:    mode,
	}

	return a.record(ctx, sess, agentName, actionType, fileAction, nil)
}

// RecordHTTPRequest records an HTTP request
func (a *Auditor) RecordHTTPRequest(ctx context.Context, sess *session.Session, agentName string, method, url string, headers map[string]string, body string, statusCode int, response string, duration time.Duration) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	action := &HTTPAction{
		Method:     method,
		URL:        url,
		Headers:    headers,
		Body:       body,
		StatusCode: statusCode,
		Response:   response,
	}

	result := &ActionResult{
		Success:  statusCode >= 200 && statusCode < 300,
		Duration: duration.String(),
	}

	return a.record(ctx, sess, agentName, ActionTypeHTTPRequest, action, result)
}

// RecordCommandExec records a command execution
func (a *Auditor) RecordCommandExec(ctx context.Context, sess *session.Session, agentName string, command string, args []string, env map[string]string, workDir string, output string, exitCode int, duration time.Duration) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	action := &CommandAction{
		Command:  command,
		Args:     args,
		Env:      env,
		WorkDir:  workDir,
		Output:   output,
		ExitCode: exitCode,
	}

	result := &ActionResult{
		Success:  exitCode == 0,
		Output:   output,
		Duration: duration.String(),
	}

	return a.record(ctx, sess, agentName, ActionTypeCommandExec, action, result)
}

// RecordSessionStart records a session start event
func (a *Auditor) RecordSessionStart(ctx context.Context, sess *session.Session, agentName string) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	return a.record(ctx, sess, agentName, ActionTypeSessionStart, map[string]any{
		"session_id": sess.ID,
		"agent":      agentName,
	}, nil)
}

// RecordSessionEnd records a session end event
func (a *Auditor) RecordSessionEnd(ctx context.Context, sess *session.Session, agentName string, reason string) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	return a.record(ctx, sess, agentName, ActionTypeSessionEnd, map[string]any{
		"session_id": sess.ID,
		"agent":      agentName,
		"reason":     reason,
	}, nil)
}

// RecordUserInput records user input
func (a *Auditor) RecordUserInput(ctx context.Context, sess *session.Session, agentName string, input string, prompt string, confirmed bool) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	action := &UserInputAction{
		Input:     input,
		Prompt:    prompt,
		Confirmed: confirmed,
	}

	return a.record(ctx, sess, agentName, ActionTypeUserInput, action, nil)
}

// RecordModelResponse records a model response
func (a *Auditor) RecordModelResponse(ctx context.Context, sess *session.Session, agentName string, model string, tokensUsed int, finishReason string) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	action := &ModelResponseAction{
		Model:        model,
		TokensUsed:   tokensUsed,
		FinishReason: finishReason,
	}

	return a.record(ctx, sess, agentName, ActionTypeModelResponse, action, nil)
}

// RecordHandoff records an agent handoff
func (a *Auditor) RecordHandoff(ctx context.Context, sess *session.Session, fromAgent, toAgent, reason string) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	action := &HandoffAction{
		FromAgent: fromAgent,
		ToAgent:   toAgent,
		Reason:    reason,
	}

	return a.record(ctx, sess, fromAgent, ActionTypeHandoff, action, nil)
}

// RecordError records an error
func (a *Auditor) RecordError(ctx context.Context, sess *session.Session, agentName string, err error, code string, recoverable bool) (*AuditRecord, error) {
	if !a.enabled {
		return nil, nil
	}

	action := &ErrorAction{
		Error:       err.Error(),
		Code:        code,
		Recoverable: recoverable,
	}

	return a.record(ctx, sess, agentName, ActionTypeError, action, &ActionResult{
		Success: false,
		Error:   err.Error(),
	})
}

// record creates and stores a new audit record
func (a *Auditor) record(ctx context.Context, sess *session.Session, agentName string, actionType ActionType, action any, result *ActionResult) (*AuditRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Generate unique ID
	id := generateID()

	// Get timestamp
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	// Get previous hash for chain integrity
	a.mu.Unlock()
	previousHash := a.getPreviousHash(sess.ID)
	a.mu.Lock()

	// Create record
	record := &AuditRecord{
		ID:           id,
		Timestamp:    timestamp,
		ActionType:   actionType,
		SessionID:    sess.ID,
		AgentName:    agentName,
		Action:       action,
		Result:       result,
		PreviousHash: previousHash,
		PublicKey:    base64.StdEncoding.EncodeToString(a.publicKey),
	}

	// Calculate hash of record (excluding signature)
	hash, err := calculateHash(record)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}
	record.Hash = hash

	// Sign the hash
	signature, err := a.sign(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign record: %w", err)
	}
	record.Signature = signature

	// Update chain
	a.sessionHash[sess.ID] = hash

	// Store record
	a.records = append(a.records, record)

	// Persist to disk
	if err := a.persistRecord(record); err != nil {
		return record, fmt.Errorf("failed to persist record: %w", err)
	}

	return record, nil
}

func (a *Auditor) getPreviousHash(sessionID string) string {
	if hash, ok := a.sessionHash[sessionID]; ok {
		return hash
	}
	return "" // Genesis block
}

func (a *Auditor) sign(data string) (string, error) {
	hash := sha256.Sum256([]byte(data))
	signature := ed25519.Sign(a.privateKey, hash[:])
	return base64.StdEncoding.EncodeToString(signature), nil
}

func calculateHash(record *AuditRecord) (string, error) {
	// Create a copy without the signature for hashing
	data := map[string]any{
		"id":            record.ID,
		"timestamp":     record.Timestamp,
		"action_type":   record.ActionType,
		"session_id":    record.SessionID,
		"agent_name":    record.AgentName,
		"action":        record.Action,
		"previous_hash": record.PreviousHash,
	}

	if record.Result != nil {
		data["result"] = record.Result
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:]), nil
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *Auditor) persistRecord(record *AuditRecord) error {
	if a.storagePath == "" {
		return nil
	}

	// Store by session ID
	sessionDir := filepath.Join(a.storagePath, record.SessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return err
	}

	// Write record to file
	filename := filepath.Join(sessionDir, fmt.Sprintf("%s.json", record.ID))
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0o600)
}

// GetRecords returns all audit records for a session
func (a *Auditor) GetRecords(sessionID string) []*AuditRecord {
	a.mu.Lock()
	defer a.mu.Unlock()

	var records []*AuditRecord
	for _, record := range a.records {
		if record.SessionID == sessionID {
			records = append(records, record)
		}
	}
	return records
}

// GetAllRecords returns all audit records
func (a *Auditor) GetAllRecords() []*AuditRecord {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]*AuditRecord(nil), a.records...)
}

// VerifyRecord verifies the signature of a single audit record
func VerifyRecord(record *AuditRecord) (bool, error) {
	// Verify hash
	calculatedHash, err := calculateHash(record)
	if err != nil {
		return false, fmt.Errorf("failed to calculate hash: %w", err)
	}
	if calculatedHash != record.Hash {
		return false, errors.New("hash mismatch - record has been tampered with")
	}

	// Decode public key
	pubKeyBytes, err := base64.StdEncoding.DecodeString(record.PublicKey)
	if err != nil {
		return false, fmt.Errorf("failed to decode public key: %w", err)
	}

	// Decode signature
	sigBytes, err := base64.StdEncoding.DecodeString(record.Signature)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	// Verify signature
	hash := sha256.Sum256([]byte(record.Hash))
	valid := ed25519.Verify(pubKeyBytes, hash[:], sigBytes)
	if !valid {
		return false, errors.New("signature verification failed")
	}

	return true, nil
}

// VerifyChain verifies the integrity of an audit chain (all records for a session)
func VerifyChain(records []*AuditRecord) (bool, error) {
	if len(records) == 0 {
		return true, nil
	}

	// Verify first record has no previous hash (genesis)
	if records[0].PreviousHash != "" {
		return false, errors.New("first record should have empty previous_hash")
	}

	// Verify each record
	var lastHash string
	for i, record := range records {
		// Verify previous hash chain
		if record.PreviousHash != lastHash {
			return false, fmt.Errorf("chain broken at record %d: previous_hash mismatch", i)
		}

		// Verify signature
		valid, err := VerifyRecord(record)
		if err != nil {
			return false, fmt.Errorf("record %d verification failed: %w", i, err)
		}
		if !valid {
			return false, fmt.Errorf("record %d signature verification failed", i)
		}

		lastHash = record.Hash
	}

	return true, nil
}

// ExportRecords exports audit records to JSON format
func (a *Auditor) ExportRecords(sessionID string) ([]byte, error) {
	records := a.GetRecords(sessionID)
	return json.MarshalIndent(records, "", "  ")
}

// PublicKey returns the public key for external verification
func (a *Auditor) PublicKey() string {
	return base64.StdEncoding.EncodeToString(a.publicKey)
}

// Close flushes any pending records to disk
func (a *Auditor) Close() error {
	if !a.enabled {
		return nil
	}

	// All records are persisted immediately, but we could add batching here
	return nil
}
