---
title: "Google Gemini"
description: "Use Gemini 2.5 Flash, Gemini 3 Pro, and other Google models with docker-agent."
permalink: /providers/google/
---

# Google Gemini

_Use Gemini 2.5 Flash, Gemini 3 Pro, and other Google models with docker-agent._

## Setup

```bash
# Set your API key
export GOOGLE_API_KEY="AI..."
```

## Configuration

### Inline

```yaml
agents:
  root:
    model: google/gemini-2.5-flash
```

### Named Model

```yaml
models:
  gemini:
    provider: google
    model: gemini-2.5-flash
    temperature: 0.5
```

## Available Models

| Model              | Best For                        |
| ------------------ | ------------------------------- |
| `gemini-3-pro`     | Most capable Gemini model       |
| `gemini-3-flash`   | Fast, efficient, good balance   |
| `gemini-2.5-flash` | Fast inference, cost-effective  |
| `gemini-2.5-pro`   | Strong reasoning, large context |

## Thinking Budget

Gemini supports two approaches depending on the model version:

<div class="callout callout-warning">
<div class="callout-title">⚠️ Different thinking formats
</div>
  <p>Gemini 2.5 uses **token-based** budgets (integers). Gemini 3 uses **level-based** budgets (strings like <code>low</code>, <code>high</code>). Make sure you use the right format for your model version.</p>

</div>

### Gemini 2.5 (Token-based)

```yaml
models:
  gemini-no-thinking:
    provider: google
    model: gemini-2.5-flash
    thinking_budget: 0 # disable thinking

  gemini-dynamic:
    provider: google
    model: gemini-2.5-flash
    thinking_budget: -1 # dynamic (model decides) — default

  gemini-fixed:
    provider: google
    model: gemini-2.5-flash
    thinking_budget: 8192 # fixed token budget
```

### Gemini 3 (Level-based)

```yaml
models:
  gemini-3-pro:
    provider: google
    model: gemini-3-pro
    thinking_budget: high # default for Pro: low | high

  gemini-3-flash:
    provider: google
    model: gemini-3-flash
    thinking_budget: medium # default for Flash: minimal | low | medium | high
```

## Built-in Tools (Grounding)

Gemini models support built-in tools that let the model access Google Search and Google Maps
directly during generation. Enable them via `provider_opts`:

```yaml
models:
  gemini-grounded:
    provider: google
    model: gemini-2.5-flash
    provider_opts:
      google_search: true
      google_maps: true
      code_execution: true
```

| Option           | Description                                          |
| ---------------- | ---------------------------------------------------- |
| `google_search`  | Enables Google Search grounding for up-to-date info  |
| `google_maps`    | Enables Google Maps grounding for location queries   |
| `code_execution` | Enables server-side code execution for computations  |
