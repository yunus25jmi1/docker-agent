package audit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/tools"
)

func TestNewAuditor(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with auditing enabled
	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
		KeyPath:     filepath.Join(tmpDir, "test_key"),
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	if !auditor.Enabled() {
		t.Error("expected auditor to be enabled")
	}

	if auditor.publicKey == nil {
		t.Error("expected public key to be generated")
	}

	if auditor.privateKey == nil {
		t.Error("expected private key to be generated")
	}
}

func TestNewAuditorDisabled(t *testing.T) {
	auditor, err := New(Config{Enabled: false})
	if err != nil {
		t.Fatalf("failed to create disabled auditor: %v", err)
	}

	if auditor.Enabled() {
		t.Error("expected auditor to be disabled")
	}
}

func TestKeyPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")

	// Create first auditor
	auditor1, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
		KeyPath:     keyPath,
	})
	if err != nil {
		t.Fatalf("failed to create first auditor: %v", err)
	}

	pubKey1 := auditor1.PublicKey()

	// Create second auditor with same key path
	auditor2, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
		KeyPath:     keyPath,
	})
	if err != nil {
		t.Fatalf("failed to create second auditor: %v", err)
	}

	pubKey2 := auditor2.PublicKey()

	// Keys should be the same
	if pubKey1 != pubKey2 {
		t.Error("expected same public key after reload")
	}
}

func TestRecordToolCall(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{
		ID: "test-session-123",
	}

	toolCall := tools.ToolCall{
		ID:   "call-abc",
		Type: "function",
		Function: tools.FunctionCall{
			Name:      "read_file",
			Arguments: `{"path": "/test/file.txt"}`,
		},
	}

	result := &tools.ToolCallResult{
		Output:  "file content",
		IsError: false,
	}

	record, err := auditor.RecordToolCall(ctx, sess, "test-agent", toolCall, result, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record tool call: %v", err)
	}

	if record == nil {
		t.Fatal("expected record to be created")
	}

	if record.ActionType != ActionTypeToolCall {
		t.Errorf("expected action type %s, got %s", ActionTypeToolCall, record.ActionType)
	}

	if record.SessionID != "test-session-123" {
		t.Errorf("expected session test-session-123, got %s", record.SessionID)
	}

	if record.AgentName != "test-agent" {
		t.Errorf("expected agent test-agent, got %s", record.AgentName)
	}

	// Verify the record
	valid, err := VerifyRecord(record)
	if err != nil {
		t.Fatalf("failed to verify record: %v", err)
	}
	if !valid {
		t.Error("record verification failed")
	}
}

func TestRecordFileOperation(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{ID: "test-session"}

	record, err := auditor.RecordFileOperation(
		ctx, sess, "test-agent",
		ActionTypeFileWrite,
		"/test/file.txt",
		"file content",
		"0644",
	)
	if err != nil {
		t.Fatalf("failed to record file operation: %v", err)
	}

	if record == nil {
		t.Fatal("expected record to be created")
	}

	valid, err := VerifyRecord(record)
	if err != nil {
		t.Fatalf("failed to verify record: %v", err)
	}
	if !valid {
		t.Error("record verification failed")
	}
}

func TestRecordSessionStartEnd(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{
		ID: "test-session",
	}

	// Record session start
	startRecord, err := auditor.RecordSessionStart(ctx, sess, "test-agent")
	if err != nil {
		t.Fatalf("failed to record session start: %v", err)
	}

	// Record session end
	endRecord, err := auditor.RecordSessionEnd(ctx, sess, "test-agent", "completed")
	if err != nil {
		t.Fatalf("failed to record session end: %v", err)
	}

	// Verify chain
	records := []*AuditRecord{startRecord, endRecord}
	valid, err := VerifyChain(records)
	if err != nil {
		t.Fatalf("failed to verify chain: %v", err)
	}
	if !valid {
		t.Error("chain verification failed")
	}
}

func TestChainIntegrity(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{ID: "test-session"}

	// Create multiple records
	var records []*AuditRecord

	for i := 0; i < 5; i++ {
		record, err := auditor.RecordToolCall(
			ctx, sess, "test-agent",
			tools.ToolCall{
				ID:   "call-" + string(rune('a'+i)),
				Type: "function",
				Function: tools.FunctionCall{
					Name: "test_tool",
				},
			},
			&tools.ToolCallResult{Output: "result"},
			time.Millisecond,
		)
		if err != nil {
			t.Fatalf("failed to record tool call %d: %v", i, err)
		}
		records = append(records, record)
	}

	// Verify chain integrity
	valid, err := VerifyChain(records)
	if err != nil {
		t.Fatalf("failed to verify chain: %v", err)
	}
	if !valid {
		t.Error("chain verification failed")
	}

	// Tamper with a record
	records[2].ActionType = "tampered"
	valid, err = VerifyChain(records)
	if err == nil {
		t.Error("expected error when verifying tampered chain")
	}
	if valid {
		t.Error("expected chain verification to fail after tampering")
	}
}

func TestVerifyRecord(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{ID: "test-session"}

	record, err := auditor.RecordToolCall(
		ctx, sess, "test-agent",
		tools.ToolCall{
			ID:   "call-abc",
			Type: "function",
			Function: tools.FunctionCall{
				Name: "test_tool",
			},
		},
		&tools.ToolCallResult{Output: "result"},
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("failed to record tool call: %v", err)
	}

	// Verify valid record
	valid, err := VerifyRecord(record)
	if err != nil {
		t.Fatalf("failed to verify record: %v", err)
	}
	if !valid {
		t.Error("expected valid record")
	}

	// Tamper with signature
	originalSig := record.Signature
	record.Signature = "tampered"
	valid, err = VerifyRecord(record)
	if err == nil {
		t.Error("expected error when verifying tampered signature")
	}
	if valid {
		t.Error("expected verification to fail with tampered signature")
	}

	// Restore signature but tamper with hash
	record.Signature = originalSig
	record.Hash = "tampered"
	valid, err = VerifyRecord(record)
	if err == nil {
		t.Error("expected error when verifying tampered hash")
	}
	if valid {
		t.Error("expected verification to fail with tampered hash")
	}
}

func TestGetRecords(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess1 := &session.Session{ID: "session-1"}
	sess2 := &session.Session{ID: "session-2"}

	// Record actions for session 1
	_, err = auditor.RecordToolCall(ctx, sess1, "agent1", tools.ToolCall{ID: "call-1"}, &tools.ToolCallResult{}, time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	_, err = auditor.RecordToolCall(ctx, sess1, "agent1", tools.ToolCall{ID: "call-2"}, &tools.ToolCallResult{}, time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Record action for session 2
	_, err = auditor.RecordToolCall(ctx, sess2, "agent2", tools.ToolCall{ID: "call-3"}, &tools.ToolCallResult{}, time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Get records for session 1
	records1 := auditor.GetRecords("session-1")
	if len(records1) != 2 {
		t.Errorf("expected 2 records for session-1, got %d", len(records1))
	}

	// Get records for session 2
	records2 := auditor.GetRecords("session-2")
	if len(records2) != 1 {
		t.Errorf("expected 1 record for session-2, got %d", len(records2))
	}
}

func TestExportRecords(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{ID: "test-session"}

	_, err = auditor.RecordToolCall(ctx, sess, "test-agent", tools.ToolCall{ID: "call-1"}, &tools.ToolCallResult{}, time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	data, err := auditor.ExportRecords("test-session")
	if err != nil {
		t.Fatalf("failed to export records: %v", err)
	}

	// Verify it's valid JSON
	var records []*AuditRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{ID: "test-session"}

	_, err = auditor.RecordToolCall(ctx, sess, "test-agent", tools.ToolCall{ID: "call-1"}, &tools.ToolCallResult{}, time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Check if file was created
	sessionDir := filepath.Join(tmpDir, "test-session")
	files, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("failed to read session directory: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}

	// Read and verify the file
	data, err := os.ReadFile(filepath.Join(sessionDir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read record file: %v", err)
	}

	var record AuditRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("failed to unmarshal record: %v", err)
	}

	if record.ActionType != ActionTypeToolCall {
		t.Errorf("expected action type %s, got %s", ActionTypeToolCall, record.ActionType)
	}
}

func TestDisabledAuditorDoesNotRecord(t *testing.T) {
	auditor, err := New(Config{Enabled: false})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	ctx := context.Background()
	sess := &session.Session{ID: "test-session"}

	record, err := auditor.RecordToolCall(ctx, sess, "test-agent", tools.ToolCall{ID: "call-1"}, &tools.ToolCallResult{}, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error from disabled auditor: %v", err)
	}

	if record != nil {
		t.Error("expected nil record from disabled auditor")
	}

	records := auditor.GetRecords("test-session")
	if len(records) != 0 {
		t.Errorf("expected no records from disabled auditor, got %d", len(records))
	}
}

func TestPublicKey(t *testing.T) {
	tmpDir := t.TempDir()

	auditor, err := New(Config{
		Enabled:     true,
		StoragePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}

	pubKey := auditor.PublicKey()
	if pubKey == "" {
		t.Error("expected non-empty public key")
	}

	// Verify it's valid base64
	_, err = base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		t.Errorf("public key is not valid base64: %v", err)
	}
}
