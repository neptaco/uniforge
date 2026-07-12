package bridge

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ParseError          = -32700
	InvalidRequest      = -32600
	MethodNotFound      = -32601
	UnityNotConnected   = -32000
	ToolNotFound        = -32001
	ServerOverloaded    = -32002
	ToolTimeout         = -32003
	UnityBusy           = -32004
	ProjectDisconnected = -32005
)

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func extractResponseID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var stringID string
	if err := json.Unmarshal(raw, &stringID); err == nil {
		return stringID
	}

	var numericID int64
	if err := json.Unmarshal(raw, &numericID); err == nil {
		return fmt.Sprintf("%d", numericID)
	}

	return strings.Trim(string(raw), "\"")
}

func newSuccessResponse(id string, result any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
}

func newErrorResponse(id string, code int, message string, data any) map[string]any {
	response := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}

	if data != nil {
		responseError := response["error"].(map[string]any)
		responseError["data"] = data
	}

	return response
}

// DaemonError is a structured error returned by the daemon.
type DaemonError struct {
	Code    int
	Message string
}

func (e *DaemonError) Error() string {
	return e.Message
}

func newRequest(id string, method string, params any) map[string]any {
	message := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}

	if params != nil {
		message["params"] = params
	}

	return message
}
