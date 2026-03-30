---
title: "Model Configuration"
description: "Complete reference for defining models with providers, parameters, and reasoning settings."
permalink: /configuration/models/
---

# Model Configuration

_Complete reference for defining models with providers, parameters, and reasoning settings._

## Full Schema

<!-- yaml-lint:skip -->
```yaml
models:
  model_name:
    provider: string # Required: openai, anthropic, google, amazon-bedrock, dmr
    model: string # Required: model identifier
    temperature: float # Optional: 0.0–1.0
    max_tokens: integer # Optional: response length limit
    top_p: float # Optional: 0.0–1.0
    frequency_penalty: float # Optional: 0.0–2.0
    presence_penalty: float # Optional: 0.0–2.0
    base_url: string # Optional: custom API endpoint
    token_key: string # Optional: env var for API token
    thinking_budget: string|int # Optional: reasoning effort
    parallel_tool_calls: boolean # Optional: allow parallel tool calls
    track_usage: boolean # Optional: track token usage
    routing: [list] # Optional: rule-based model routing
    provider_opts: # Optional: provider-specific options
      key: value
```

## Properties Reference

| Property              | Type       | Required | Description                                                                           |
| --------------------- | ---------- | -------- | ------------------------------------------------------------------------------------- |
| `provider`            | string     | ✓        | Provider: `openai`, `anthropic`, `google`, `amazon-bedrock`, `dmr`, `mistral`, `xai`  |
| `model`               | string     | ✓        | Model name (e.g., `gpt-4o`, `claude-sonnet-4-0`, `gemini-2.5-flash`)                  |
| `temperature`         | float      | ✗        | Randomness. `0.0` = deterministic, `1.0` = creative                                   |
| `max_tokens`          | int        | ✗        | Maximum response length in tokens                                                     |
| `top_p`               | float      | ✗        | Nucleus sampling threshold                                                            |
| `frequency_penalty`   | float      | ✗        | Penalize repeated tokens (0.0–2.0)                                                    |
| `presence_penalty`    | float      | ✗        | Encourage topic diversity (0.0–2.0)                                                   |
| `base_url`            | string     | ✗        | Custom API endpoint URL (for self-hosted or proxied endpoints)                        |
| `token_key`           | string     | ✗        | Environment variable name containing the API token (overrides provider default)       |
| `thinking_budget`     | string/int | ✗        | Reasoning effort control                                                              |
| `parallel_tool_calls` | boolean    | ✗        | Allow model to call multiple tools at once                                            |
| `track_usage`         | boolean    | ✗        | Track and report token usage for this model                                           |
| `routing`             | array      | ✗        | Rule-based routing to different models. See [Model Routing]({{ '/configuration/routing/' | relative_url }}). |
| `provider_opts`       | object     | ✗        | Provider-specific options (see provider pages)                                        |

## Thinking Budget

Control how much reasoning the model does before responding. This varies by provider:

### OpenAI

Uses effort levels as strings:

```yaml
models:
  gpt:
    provider: openai
    model: gpt-5-mini
    thinking_budget: low # minimal | low | medium | high
```

### Anthropic

Uses an integer token budget (1024–32768):

```yaml
models:
  claude:
    provider: anthropic
    model: claude-sonnet-4-5
    thinking_budget: 16384 # must be < max_tokens
```

### Google Gemini 2.5

Uses an integer token budget. `0` disables, `-1` lets the model decide:

```yaml
models:
  gemini:
    provider: google
    model: gemini-2.5-flash
    thinking_budget: -1 # dynamic (default)
```

### Google Gemini 3

Uses effort levels like OpenAI:

```yaml
models:
  gemini3:
    provider: google
    model: gemini-3-flash
    thinking_budget: medium # minimal | low | medium | high
```

### Disabling Thinking

Works for all providers:

```yaml
thinking_budget: none # or 0
```

## Interleaved Thinking

For Anthropic and Bedrock Claude models, interleaved thinking allows tool calls during model reasoning. This is enabled by default:

```yaml
models:
  claude:
    provider: anthropic
    model: claude-sonnet-4-5
    # interleaved_thinking defaults to true
    provider_opts:
      interleaved_thinking: false # disable if needed
```

## Examples by Provider

```yaml
models:
  # OpenAI
  gpt:
    provider: openai
    model: gpt-5-mini

  # Anthropic
  claude:
    provider: anthropic
    model: claude-sonnet-4-0
    max_tokens: 64000

  # Google Gemini
  gemini:
    provider: google
    model: gemini-2.5-flash
    temperature: 0.5

  # AWS Bedrock
  bedrock:
    provider: amazon-bedrock
    model: global.anthropic.claude-sonnet-4-5-20250929-v1:0
    provider_opts:
      region: us-east-1

  # Docker Model Runner (local)
  local:
    provider: dmr
    model: ai/qwen3
    max_tokens: 8192
```

For detailed provider setup, see the [Model Providers]({{ '/providers/overview/' | relative_url }}) section.

## Custom Endpoints

Use `base_url` to point to custom or self-hosted endpoints:

```yaml
models:
  # Azure OpenAI
  azure_gpt:
    provider: openai
    model: gpt-4o
    base_url: https://my-resource.openai.azure.com/openai/deployments/gpt-4o
    token_key: AZURE_OPENAI_API_KEY

  # Self-hosted vLLM
  local_llama:
    provider: openai # vLLM is OpenAI-compatible
    model: meta-llama/Llama-3.2-3B-Instruct
    base_url: http://localhost:8000/v1

  # Proxy or gateway
  proxied:
    provider: openai
    model: gpt-4o
    base_url: https://proxy.internal.company.com/openai/v1
    token_key: INTERNAL_API_KEY
```

See [Local Models]({{ '/providers/local/' | relative_url }}) for more examples of custom endpoints.
