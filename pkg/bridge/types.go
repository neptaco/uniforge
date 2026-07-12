package bridge

const (
	ProtocolVersion = 2
	PackageVersion  = "0.1.0"
)

// DaemonMeta is bridge-specific metadata stored in daemon.Info.Metadata.
type DaemonMeta struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Version         string `json:"version"`
}

type ToolDefinition struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
}

type ProjectInfo struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	GitRoot    string           `json:"gitRoot,omitempty"`
	Connected  bool             `json:"connected"`
	SchemaHash string           `json:"schemaHash,omitempty"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
}

type ClientRegisterResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	PackageVersion  string `json:"packageVersion"`
	BuildTimestamp  int64  `json:"buildTimestamp"`
	Compatible      bool   `json:"compatible"`
	Warning         string `json:"warning,omitempty"`
}

type ClientListProjectsResult struct {
	Projects []ProjectInfo `json:"projects"`
}

type ClientToolCallResult struct {
	Success bool   `json:"success"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}
