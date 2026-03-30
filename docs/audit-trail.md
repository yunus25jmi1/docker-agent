# Audit Trail Feature for Docker Agent

## Overview

The Audit Trail feature provides cryptographic, tamper-proof logging of all agent actions for governance and compliance purposes. Each tool call, file operation, HTTP request, or command execution is recorded with a cryptographic signature that can be independently verified.

## Features

- **Cryptographic Signing**: Each audit record is signed using Ed25519 digital signatures
- **Chain Integrity**: Records are chained together using hash links (like a blockchain)
- **Tamper-Proof**: Any modification to audit records can be detected
- **Independent Verification**: Records can be verified without trusting the agent
- **Configurable Recording**: Fine-grained control over what actions to record
- **Privacy Controls**: Options to exclude sensitive input/output content

## Configuration

Enable auditing in your agent configuration file:

```yaml
audit:
  enabled: true
  storage_path: ./audit-logs
  key_path: ./audit-key
  record_tool_calls: true
  record_file_ops: true
  record_http: true
  record_commands: true
  record_sessions: true
  include_input_content: false
  include_output_content: false
```

### Configuration Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable audit trail recording |
| `storage_path` | string | `~/.cagent/audit` | Directory to store audit records |
| `key_path` | string | `{storage_path}/audit_key` | Path to the Ed25519 signing key |
| `record_tool_calls` | boolean | `true` | Record tool call actions |
| `record_file_ops` | boolean | `true` | Record file read/write/delete operations |
| `record_http` | boolean | `true` | Record HTTP requests and responses |
| `record_commands` | boolean | `true` | Record shell command executions |
| `record_sessions` | boolean | `true` | Record session start/end events |
| `include_input_content` | boolean | `false` | Include user input in audit records |
| `include_output_content` | boolean | `false` | Include tool output in audit records |

## Audit Record Structure

Each audit record contains:

```json
{
  "id": "unique-record-id",
  "timestamp": "2026-03-28T12:00:00Z",
  "action_type": "tool_call",
  "session_id": "session-123",
  "agent_name": "my-agent",
  "action": {
    "tool_name": "read_file",
    "tool_type": "function",
    "tool_call_id": "call-abc",
    "arguments": {"path": "/test/file.txt"}
  },
  "result": {
    "success": true,
    "output": "file content",
    "duration": "100ms"
  },
  "previous_hash": "sha256-of-previous-record",
  "hash": "sha256-of-this-record",
  "signature": "ed25519-signature",
  "public_key": "base64-encoded-public-key"
}
```

## Action Types

The audit system records the following action types:

- `tool_call`: Tool invocations with arguments and results
- `file_read`: File read operations
- `file_write`: File write operations
- `file_delete`: File deletion operations
- `http_request`: HTTP requests with method, URL, and response
- `command_exec`: Shell command executions
- `session_start`: Session initialization
- `session_end`: Session termination
- `user_input`: User input (if enabled)
- `model_response`: LLM response metadata
- `handoff`: Agent handoff events
- `error`: Error conditions

## Verification

### Programmatic Verification

Use the audit package to verify records:

```go
import "github.com/docker/docker-agent/pkg/audit"

// Verify a single record
valid, err := audit.VerifyRecord(record)
if err != nil {
    log.Fatalf("Verification failed: %v", err)
}

// Verify an entire chain (all records for a session)
records := getRecordsForSession(sessionID)
valid, err := audit.VerifyChain(records)
if err != nil {
    log.Fatalf("Chain verification failed: %v", err)
}
```

### Manual Verification

1. Extract the public key from any audit record
2. Verify the signature using the public key
3. Verify the hash matches the record content
4. Verify the chain by checking previous_hash links

## Storage

Audit records are stored as JSON files organized by session ID:

```
audit-logs/
├── session-abc123/
│   ├── record-001.json
│   ├── record-002.json
│   └── ...
├── session-def456/
│   └── ...
└── audit-key (signing key)
```

## Security Considerations

### Key Management

- The signing key is automatically generated on first run
- Store the key securely with appropriate file permissions (0600)
- Backup the key for long-term verification capability
- Consider using HSM or secure key storage for production deployments

### Privacy

- By default, input and output content is NOT recorded
- Enable content recording only when necessary for compliance
- Consider encrypting audit storage at rest
- Implement appropriate access controls for audit logs

### Integrity

- The chain structure makes tampering evident
- Each record's hash depends on the previous record
- Signatures prevent unauthorized modifications
- Regular verification recommended for compliance

## Compliance Use Cases

### SOC 2 Compliance

- Demonstrates control over AI agent actions
- Provides audit trail for access reviews
- Enables incident investigation

### GDPR Compliance

- Records data access operations
- Supports data subject access requests
- Enables privacy impact assessments

### Financial Regulations

- Provides transaction audit trail
- Supports regulatory examinations
- Enables forensic analysis

## Example Usage

See `examples/audit-example.yaml` for a complete example configuration.

## API Reference

### Creating an Auditor

```go
auditor, err := audit.New(audit.Config{
    Enabled:     true,
    StoragePath: "/var/log/audit",
    KeyPath:     "/secure/keys/audit-key",
})
```

### Recording Actions

```go
// Tool call
record, err := auditor.RecordToolCall(ctx, sess, agentName, toolCall, result, duration)

// File operation
record, err := auditor.RecordFileOperation(ctx, sess, agentName, actionType, path, content, mode)

// HTTP request
record, err := auditor.RecordHTTPRequest(ctx, sess, agentName, method, url, headers, body, statusCode, response, duration)

// Session events
record, err := auditor.RecordSessionStart(ctx, sess, agentName)
record, err := auditor.RecordSessionEnd(ctx, sess, agentName, reason)
```

### Exporting Records

```go
// Export all records for a session
data, err := auditor.ExportRecords(sessionID)

// Get public key for verification
pubKey := auditor.PublicKey()
```

## Troubleshooting

### Audit Records Not Being Created

1. Check that `audit.enabled` is set to `true`
2. Verify the storage directory is writable
3. Check logs for initialization errors

### Verification Failures

1. Ensure you're using the correct public key
2. Check that records haven't been modified
3. Verify the chain hasn't been broken

### Performance Impact

- Auditing adds minimal overhead (<1ms per record)
- Consider async writing for high-volume scenarios
- Monitor storage growth in long-running sessions

## Future Enhancements

- Encrypted audit storage
- Remote audit log shipping
- Integration with SIEM systems
- Audit log rotation and retention policies
- Real-time alerting on specific actions
