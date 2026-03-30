package rag

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/environment"
	"github.com/docker/docker-agent/pkg/model/provider"
	"github.com/docker/docker-agent/pkg/rag/rerank"
	"github.com/docker/docker-agent/pkg/rag/strategy"
	"github.com/docker/docker-agent/pkg/rag/types"
)

// ManagersBuildConfig contains dependencies needed to build RAG managers from config.
type ManagersBuildConfig struct {
	ParentDir     string
	ModelsGateway string
	Env           environment.Provider
	Models        map[string]latest.ModelConfig // Model configurations from config
}

// NewManager constructs a single RAG manager from a RAGConfig.
func NewManager(
	ctx context.Context,
	ragName string,
	ragCfg *latest.RAGConfig,
	buildCfg ManagersBuildConfig,
) (*Manager, error) {
	if ragCfg == nil {
		return nil, fmt.Errorf("nil RAG config for %q", ragName)
	}

	// Validate that we have at least one strategy
	if len(ragCfg.Strategies) == 0 {
		return nil, fmt.Errorf("no strategies configured for RAG %q", ragName)
	}

	// Build context for strategy builders
	strategyBuildCtx := strategy.BuildContext{
		RAGName:       ragName,
		ParentDir:     buildCfg.ParentDir,
		SharedDocs:    GetAbsolutePaths(buildCfg.ParentDir, ragCfg.Docs),
		Models:        buildCfg.Models,
		Env:           buildCfg.Env,
		ModelsGateway: buildCfg.ModelsGateway,
		RespectVCS:    ragCfg.GetRespectVCS(),
	}

	strategyConfigs, strategyEvents, err := buildStrategyConfigs(ctx, *ragCfg, strategyBuildCtx, ragName)
	if err != nil {
		return nil, fmt.Errorf("failed to build strategy configs for RAG %q: %w", ragName, err)
	}

	managerCfg, err := buildManagerConfig(ctx, *ragCfg, buildCfg, strategyConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to build manager config for RAG %q: %w", ragName, err)
	}

	manager, err := New(ctx, ragName, managerCfg, strategyEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to create RAG manager %q: %w", ragName, err)
	}

	strategyNames := make([]string, len(strategyConfigs))
	for i, sc := range strategyConfigs {
		strategyNames[i] = sc.Name
	}
	slog.Debug("Created RAG manager",
		"name", ragName,
		"strategies", strategyNames,
		"docs", len(managerCfg.Docs))

	return manager, nil
}

// buildManagerConfig constructs a rag.Manager Config from the configuration and strategies.
func buildManagerConfig(
	ctx context.Context,
	ragCfg latest.RAGConfig,
	buildCfg ManagersBuildConfig,
	strategyConfigs []strategy.Config,
) (Config, error) {
	results := ResultsConfig{
		Limit:             ragCfg.Results.Limit,
		Deduplicate:       ragCfg.Results.Deduplicate,
		IncludeScore:      ragCfg.Results.IncludeScore,
		ReturnFullContent: ragCfg.Results.ReturnFullContent,
	}

	// Build reranking config if configured
	if ragCfg.Results.Reranking != nil {
		slog.Debug("Building reranking configuration",
			"model", ragCfg.Results.Reranking.Model,
			"top_k", ragCfg.Results.Reranking.TopK,
			"threshold", ragCfg.Results.Reranking.Threshold)

		rerankingCfg, err := buildRerankingConfig(ctx, ragCfg.Results.Reranking, buildCfg, results.Limit)
		if err != nil {
			slog.Error("Failed to build reranking config",
				"model", ragCfg.Results.Reranking.Model,
				"error", err)
			return Config{}, fmt.Errorf("failed to build reranking config: %w", err)
		}
		results.RerankingConfig = rerankingCfg
		slog.Debug("Reranking configuration built successfully",
			"model", ragCfg.Results.Reranking.Model)
	}

	fusionCfg := buildManagerFusionConfig(ragCfg, strategyConfigs)

	return Config{
		Tool: ToolConfig{
			Name:        ragCfg.Tool.Name,
			Description: ragCfg.Tool.Description,
			Instruction: ragCfg.Tool.Instruction,
		},
		Docs:            GetAbsolutePaths(buildCfg.ParentDir, ragCfg.Docs),
		Results:         results,
		FusionConfig:    fusionCfg,
		StrategyConfigs: strategyConfigs,
	}, nil
}

// buildRerankingConfig constructs a RerankingConfig from the configuration.
func buildRerankingConfig(
	ctx context.Context,
	rerankCfg *latest.RAGRerankingConfig,
	buildCfg ManagersBuildConfig,
	globalLimit int,
) (*RerankingConfig, error) {
	if rerankCfg == nil {
		return nil, nil
	}

	if rerankCfg.Model == "" {
		slog.Error("Reranking model name is empty")
		return nil, errors.New("reranking model is required")
	}

	slog.Debug("Resolving reranking model",
		"model_ref", rerankCfg.Model)

	// Resolve model config - check if it's a reference to a defined model or inline
	modelCfg, err := resolveModelConfig(rerankCfg.Model, buildCfg)
	if err != nil {
		slog.Error("Failed to resolve reranking model",
			"model_ref", rerankCfg.Model,
			"error", err)
		return nil, fmt.Errorf("failed to resolve reranking model %q: %w", rerankCfg.Model, err)
	}

	slog.Debug("Resolved reranking model config",
		"provider", modelCfg.Provider,
		"model", modelCfg.Model)

	// Create provider for reranking model
	rerankProvider, err := provider.New(ctx, modelCfg, buildCfg.Env)
	if err != nil {
		slog.Error("Failed to create reranking provider",
			"provider", modelCfg.Provider,
			"model", modelCfg.Model,
			"error", err)
		return nil, fmt.Errorf("failed to create reranking provider: %w", err)
	}

	slog.Debug("Created reranking provider",
		"provider_id", rerankProvider.ID())

	// Determine effective TopK:
	// - If user provided a positive top_k, respect it.
	// - Otherwise, default to the global results limit when set.
	//   This avoids reranking unbounded result sets while still
	//   using a sensible, user-controlled cap.
	effectiveTopK := rerankCfg.TopK
	if effectiveTopK <= 0 && globalLimit > 0 {
		effectiveTopK = globalLimit
	}

	// Create reranker
	reranker, err := rerank.NewLLMReranker(rerank.Config{
		Model:     rerankProvider,
		TopK:      effectiveTopK,
		Threshold: rerankCfg.Threshold,
		Criteria:  rerankCfg.Criteria,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create reranker: %w", err)
	}

	slog.Info("Built reranking configuration successfully",
		"model", rerankCfg.Model,
		"provider_id", rerankProvider.ID(),
		"top_k", effectiveTopK,
		"threshold", rerankCfg.Threshold,
		"has_criteria", rerankCfg.Criteria != "")

	return &RerankingConfig{
		Reranker:  reranker,
		TopK:      effectiveTopK,
		Threshold: rerankCfg.Threshold,
	}, nil
}

// resolveModelConfig resolves a model name to a ModelConfig
// Handles both inline model references (e.g., "dmr/model-name") and defined model names
func resolveModelConfig(modelName string, buildCfg ManagersBuildConfig) (*latest.ModelConfig, error) {
	// Check if it's an inline model reference (contains a '/')
	if modelName != "" {
		parts := splitModelRef(modelName)
		if len(parts) == 2 {
			// Inline model reference like "dmr/hf.co/model" or "openai/gpt-5"
			slog.Debug("Using inline model reference",
				"provider", parts[0],
				"model", parts[1])
			return &latest.ModelConfig{
				Provider: parts[0],
				Model:    parts[1],
			}, nil
		}
	}

	// Try to find model in defined models
	if modelCfg, exists := buildCfg.Models[modelName]; exists {
		slog.Debug("Using defined model from config",
			"model_name", modelName,
			"provider", modelCfg.Provider,
			"model", modelCfg.Model)
		return &modelCfg, nil
	}

	slog.Error("Model not found in configuration",
		"model_name", modelName,
		"available_models", getModelNames(buildCfg.Models))
	return nil, fmt.Errorf("model %q not found in configuration", modelName)
}

// getModelNames extracts model names from the models map for logging
func getModelNames(models map[string]latest.ModelConfig) []string {
	return slices.Collect(maps.Keys(models))
}

// splitModelRef splits a model reference into provider and model parts
func splitModelRef(ref string) []string {
	// Handle common patterns: "provider/model"
	for i := range len(ref) {
		if ref[i] == '/' {
			return []string{ref[:i], ref[i+1:]}
		}
	}
	return []string{ref}
}

// buildStrategyConfigs builds the strategy configs for the RAG.
// Returns a slice of strategy configs and a channel for receiving strategy events.
func buildStrategyConfigs(
	ctx context.Context,
	ragCfg latest.RAGConfig,
	strategyBuildCtx strategy.BuildContext,
	ragName string,
) ([]strategy.Config, chan types.Event, error) {
	// Create event channel for strategies to emit events.
	// This channel is shared with the manager, which exposes it directly to callers.
	// Use generous buffer to prevent blocking during heavy indexing.
	strategyEvents := make(chan types.Event, 500)

	// Build all strategies for this RAG source
	var strategyConfigs []strategy.Config
	for _, strategyCfg := range ragCfg.Strategies {
		builtStrategy, err := strategy.BuildStrategy(ctx, strategyCfg, strategyBuildCtx, strategyEvents)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build strategy %q for RAG %q: %w",
				strategyCfg.Type, ragName, err)
		}

		// Copy the built strategy config so callers can't mutate internal state.
		strategyConfigs = append(strategyConfigs, *builtStrategy)
	}

	return strategyConfigs, strategyEvents, nil
}

// buildManagerFusionConfig constructs a FusionConfig from the configuration.
// Returns nil if only one strategy is configured (no fusion needed).
func buildManagerFusionConfig(
	ragCfg latest.RAGConfig,
	strategyConfigs []strategy.Config,
) *FusionConfig {
	// Only use fusion if multiple strategies
	if len(strategyConfigs) <= 1 {
		return nil
	}

	fusionStrategy := "rrf" // Default to Reciprocal Rank Fusion
	fusionK := 60
	var fusionWeights map[string]float64

	if ragCfg.Results.Fusion != nil {
		if ragCfg.Results.Fusion.Strategy != "" {
			fusionStrategy = ragCfg.Results.Fusion.Strategy
		}
		if ragCfg.Results.Fusion.K > 0 {
			fusionK = ragCfg.Results.Fusion.K
		}
		fusionWeights = ragCfg.Results.Fusion.Weights
	}

	return &FusionConfig{
		Strategy: fusionStrategy,
		K:        fusionK,
		Weights:  fusionWeights,
	}
}
