package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

const defaultRefreshInterval = 2 * time.Second

// ServerOptions configures the stdio MCP server surface.
type ServerOptions struct {
	Name            string
	Version         string
	Instructions    string
	Logger          *slog.Logger
	RefreshInterval time.Duration
}

// Server exposes runtime tools over the official MCP Go SDK.
type Server struct {
	runtime         Runtime
	implementation  *mcpsdk.Implementation
	logger          *slog.Logger
	instructions    string
	refreshInterval time.Duration
}

type registeredTool struct {
	metadata ToolMetadata
}

// NewServer creates a new stdio MCP server wrapper.
func NewServer(runtime Runtime, options ServerOptions) *Server {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	name := options.Name
	if name == "" {
		name = "uniforge"
	}

	refreshInterval := options.RefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}

	return &Server{
		runtime: runtime,
		implementation: &mcpsdk.Implementation{
			Name:    name,
			Version: options.Version,
		},
		logger:          logger,
		instructions:    options.Instructions,
		refreshInterval: refreshInterval,
	}
}

// Run starts serving tools over the provided transport.
func (s *Server) Run(ctx context.Context, transport mcpsdk.Transport) error {
	server := mcpsdk.NewServer(s.implementation, &mcpsdk.ServerOptions{
		Instructions: s.instructions,
		Logger:       s.logger,
	})

	state := &serverState{
		runtime:   s.runtime,
		server:    server,
		logger:    s.logger,
		toolsByID: map[string]registeredTool{},
	}

	if err := state.refresh(ctx); err != nil {
		return err
	}

	go state.refreshLoop(ctx, s.refreshInterval)

	return server.Run(ctx, transport)
}

type serverState struct {
	runtime Runtime
	server  *mcpsdk.Server
	logger  *slog.Logger

	mu        sync.Mutex
	toolsByID map[string]registeredTool
}

func (s *serverState) refreshLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refresh(ctx); err != nil {
				s.logger.Warn("failed to refresh MCP tools", "error", err)
			}
		}
	}
}

func (s *serverState) refresh(ctx context.Context) error {
	tools, err := s.runtime.ListToolMetadata(ctx, ListToolsOptions{})
	if err != nil {
		return err
	}

	next := make(map[string]registeredTool, len(tools))
	for _, tool := range tools {
		next[tool.Name] = registeredTool{metadata: tool}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for name := range s.toolsByID {
		if _, exists := next[name]; !exists {
			s.server.RemoveTools(name)
		}
	}

	for name, tool := range next {
		current, exists := s.toolsByID[name]
		if exists && toolMetadataEqual(current.metadata, tool.metadata) {
			continue
		}

		s.server.AddTool(tool.metadata.toSDKTool(), s.buildHandler(tool.metadata))
	}

	s.toolsByID = next
	return nil
}

func (s *serverState) buildHandler(metadata ToolMetadata) mcpsdk.ToolHandler {
	inputSchema := resolveSchema(metadata.inputSchemaForMCP())
	outputSchema := resolveSchema(metadata.OutputSchema)

	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		args, err := decodeArguments(req.Params.Arguments, inputSchema)
		if err != nil {
			return toolErrorResult(fmt.Errorf("invalid input: %w", err)), nil
		}

		execOptions := ExecuteToolOptions{}
		if len(metadata.Sources) == 1 {
			execOptions.Project = metadata.Sources[0].ID
		} else if projectID, ok := args["project_id"].(string); ok && projectID != "" {
			execOptions.Project = projectID
		}

		result, err := s.runtime.ExecuteTool(ctx, metadata.Name, args, execOptions)
		if err != nil {
			return toolErrorResult(err), nil
		}

		if !result.Success {
			if result.Error == "" {
				return toolErrorResult(fmt.Errorf("tool %s failed", metadata.Name)), nil
			}
			return toolErrorResult(fmt.Errorf("%s", result.Error)), nil
		}

		if err := validateStructuredResult(result.Result, outputSchema); err != nil {
			return toolErrorResult(fmt.Errorf("invalid output: %w", err)), nil
		}

		return buildCallToolResult(result.Result), nil
	}
}

func (m ToolMetadata) toSDKTool() *mcpsdk.Tool {
	tool := &mcpsdk.Tool{
		Name:         m.Name,
		Description:  m.Description,
		InputSchema:  m.inputSchemaForMCP(),
		OutputSchema: nilIfEmptyMap(m.OutputSchema),
		Annotations:  convertToolAnnotations(m.Annotations),
	}

	meta := mcpsdk.Meta{
		"uniforge": map[string]any{
			"sources":      m.sourcesMeta(),
			"hasConflicts": m.HasConflicts,
		},
	}
	if len(m.Annotations) > 0 {
		meta["uniforgeBridgeAnnotations"] = maps.Clone(m.Annotations)
	}
	tool.Meta = meta

	return tool
}

func (m ToolMetadata) inputSchemaForMCP() map[string]any {
	schema := cloneMap(ensureObjectSchema(m.InputSchema).(map[string]any))
	if len(m.Sources) <= 1 {
		return schema
	}

	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
	}
	properties["project_id"] = map[string]any{
		"type":        "string",
		"description": "Project ID to target when multiple connected Unity projects provide the same tool",
	}
	schema["properties"] = properties
	return schema
}

func (m ToolMetadata) sourcesMeta() []map[string]any {
	result := make([]map[string]any, 0, len(m.Sources))
	for _, source := range m.Sources {
		result = append(result, map[string]any{
			"id":   source.ID,
			"name": source.Name,
		})
	}
	return result
}

func convertToolAnnotations(raw map[string]any) *mcpsdk.ToolAnnotations {
	if len(raw) == 0 {
		return nil
	}

	annotations := &mcpsdk.ToolAnnotations{}
	hasValues := false

	if title, ok := stringAnnotation(raw, "title"); ok {
		annotations.Title = title
		hasValues = true
	}
	if readOnly, ok := boolAnnotation(raw, "readOnlyHint"); ok {
		annotations.ReadOnlyHint = readOnly
		hasValues = true
	}
	if idempotent, ok := boolAnnotation(raw, "idempotentHint"); ok {
		annotations.IdempotentHint = idempotent
		hasValues = true
	}
	if destructive, ok := boolAnnotation(raw, "destructiveHint"); ok {
		annotations.DestructiveHint = &destructive
		hasValues = true
	}
	if openWorld, ok := boolAnnotation(raw, "openWorldHint"); ok {
		annotations.OpenWorldHint = &openWorld
		hasValues = true
	}

	if !hasValues {
		return nil
	}
	return annotations
}

func boolAnnotation(raw map[string]any, key string) (bool, bool) {
	value, ok := raw[key]
	if !ok {
		return false, false
	}
	flag, ok := value.(bool)
	return flag, ok
}

func stringAnnotation(raw map[string]any, key string) (string, bool) {
	value, ok := raw[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func ensureObjectSchema(schema map[string]any) any {
	if len(schema) == 0 {
		return map[string]any{"type": "object"}
	}
	return maps.Clone(schema)
}

func nilIfEmptyMap(schema map[string]any) any {
	if len(schema) == 0 {
		return nil
	}
	return maps.Clone(schema)
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}

	data, err := json.Marshal(input)
	if err != nil {
		return maps.Clone(input)
	}

	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		return maps.Clone(input)
	}

	return output
}

func resolveSchema(raw map[string]any) *jsonschema.Resolved {
	if len(raw) == 0 {
		return nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil
	}

	return resolved
}

func decodeArguments(raw json.RawMessage, schema *jsonschema.Resolved) (map[string]any, error) {
	args := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, err
		}
	}

	if schema != nil {
		if err := schema.ApplyDefaults(&args); err != nil {
			return nil, err
		}
		if err := schema.Validate(args); err != nil {
			return nil, err
		}
	}

	return args, nil
}

func validateStructuredResult(payload any, schema *jsonschema.Resolved) error {
	if schema == nil || payload == nil {
		return nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	return schema.Validate(value)
}

func buildCallToolResult(payload any) *mcpsdk.CallToolResult {
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{},
	}
	if payload == nil {
		return result
	}

	if text, ok := payload.(string); ok {
		result.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: text}}
		return result
	}

	data, err := yaml.Marshal(payload)
	if err != nil {
		result.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: fmt.Sprintf("%v", payload)}}
		return result
	}

	result.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}

	var structured map[string]any
	jsonData, err := json.Marshal(payload)
	if err == nil && json.Unmarshal(jsonData, &structured) == nil {
		result.StructuredContent = structured
	}

	return result
}

func toolErrorResult(err error) *mcpsdk.CallToolResult {
	result := &mcpsdk.CallToolResult{}
	result.SetError(err)
	return result
}

func toolMetadataEqual(a, b ToolMetadata) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
