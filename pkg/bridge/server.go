package bridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxPendingRequests = 1000
	maxBusyRetries     = 10
)

type unityRegisterParams struct {
	ProjectID         string           `json:"projectId"`
	ProjectName       string           `json:"projectName"`
	GitRoot           string           `json:"gitRoot,omitempty"`
	Tools             []ToolDefinition `json:"tools"`
	PendingRequestIDs []string         `json:"pendingRequestIds,omitempty"`
}

type unityToolsUpdateParams struct {
	ProjectID string           `json:"projectId"`
	Tools     []ToolDefinition `json:"tools"`
}

type unityBusyParams struct {
	RequestID    any    `json:"requestId"`
	RetryAfterMS int    `json:"retry_after_ms"`
	Reason       string `json:"reason,omitempty"`
}

type clientRegisterParams struct {
	ClientID        string `json:"clientId"`
	ProtocolVersion *int   `json:"protocolVersion,omitempty"`
	PackageVersion  string `json:"packageVersion,omitempty"`
	BuildTimestamp  int64  `json:"buildTimestamp,omitempty"`
}

type clientToolCallParams struct {
	ProjectID string         `json:"projectId,omitempty"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	TimeoutMS int            `json:"timeoutMs,omitempty"`
}

type clientListProjectsParams struct {
	IncludeTools bool `json:"includeTools"`
}

type unityToolResultEnvelope struct {
	Success bool   `json:"success"`
	Pending bool   `json:"pending,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

type serverConnection struct {
	id          string
	conn        net.Conn
	reader      *bufio.Reader
	sendMu      sync.Mutex
	kind        string
	clientID    string
	projectID   string
	projectName string
	gitRoot     string
	tools       []ToolDefinition
	schemaHash  string
}

type pendingRequest struct {
	daemonRequestID           string
	clientRequestID           string
	clientConn                *serverConnection
	targetConn                *serverConnection
	targetProjectID           string
	tool                      string
	args                      map[string]any
	timeout                   time.Duration
	timeoutTimer              *time.Timer
	busyRetryTimer            *time.Timer
	retryCount                int
	awaitingReconnection      bool
	awaitingUnityContinuation bool
}

type Server struct {
	listener      net.Listener
	mu            sync.Mutex
	unityConns    map[string]*serverConnection
	clientConns   map[string]*serverConnection
	pending       map[string]*pendingRequest
	nextConnID    uint64
	nextRequestID uint64
}

func NewServer() *Server {
	return &Server{
		unityConns:  map[string]*serverConnection{},
		clientConns: map[string]*serverConnection{},
		pending:     map[string]*pendingRequest{},
	}
}

// Serve accepts connections on the given listener until the listener is closed.
// Call [Stop] from another goroutine to shut down.
func (s *Server) Serve(listener net.Listener) error {
	s.listener = listener
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		connection := &serverConnection{
			id:     fmt.Sprintf("conn-%d", atomic.AddUint64(&s.nextConnID, 1)),
			conn:   conn,
			reader: bufio.NewReader(conn),
		}

		go s.handleConnection(connection)
	}
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pending := range s.pending {
		if pending.timeoutTimer != nil {
			pending.timeoutTimer.Stop()
		}
		if pending.busyRetryTimer != nil {
			pending.busyRetryTimer.Stop()
		}
	}

	for _, conn := range s.unityConns {
		_ = conn.conn.Close()
	}
	for _, conn := range s.clientConns {
		_ = conn.conn.Close()
	}

	if s.listener != nil {
		return s.listener.Close()
	}

	return nil
}

func (s *Server) handleConnection(conn *serverConnection) {
	defer s.handleDisconnect(conn)

	for {
		line, err := conn.reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}

		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}

		var envelope rpcEnvelope
		if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
			_ = conn.send(newErrorResponse("", ParseError, "invalid JSON", nil))
			continue
		}

		if envelope.Method == "" {
			s.handleResponse(conn, envelope)
			continue
		}

		id := extractResponseID(envelope.ID)
		if id == "" {
			s.handleNotification(conn, envelope)
			continue
		}

		s.handleRequest(conn, id, envelope)
	}
}

func (s *Server) handleRequest(conn *serverConnection, requestID string, envelope rpcEnvelope) {
	switch envelope.Method {
	case "unity.register":
		var params unityRegisterParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			_ = conn.send(newErrorResponse(requestID, InvalidRequest, "invalid unity.register params", nil))
			return
		}
		s.handleUnityRegister(conn, params)
		_ = conn.send(newSuccessResponse(requestID, map[string]any{"success": true}))
	case "client.register":
		var params clientRegisterParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			_ = conn.send(newErrorResponse(requestID, InvalidRequest, "invalid client.register params", nil))
			return
		}
		s.handleClientRegister(conn, params)
		result := ClientRegisterResult{
			ProtocolVersion: ProtocolVersion,
			PackageVersion:  PackageVersion,
			BuildTimestamp:  0,
			Compatible:      params.ProtocolVersion == nil || *params.ProtocolVersion == ProtocolVersion,
		}
		if !result.Compatible {
			result.Warning = fmt.Sprintf("Protocol version mismatch: client=%d, daemon=%d", *params.ProtocolVersion, ProtocolVersion)
		}
		_ = conn.send(newSuccessResponse(requestID, result))
	case "client.listProjects":
		var params clientListProjectsParams
		_ = json.Unmarshal(envelope.Params, &params)
		_ = conn.send(newSuccessResponse(requestID, s.buildProjectsResult(params.IncludeTools)))
	case "client.toolCall":
		var params clientToolCallParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			_ = conn.send(newErrorResponse(requestID, InvalidRequest, "invalid client.toolCall params", nil))
			return
		}
		s.handleClientToolCall(conn, requestID, params)
	default:
		_ = conn.send(newErrorResponse(requestID, MethodNotFound, fmt.Sprintf("unknown method: %s", envelope.Method), nil))
	}
}

func (s *Server) handleNotification(conn *serverConnection, envelope rpcEnvelope) {
	switch envelope.Method {
	case "unity.toolsUpdate":
		var params unityToolsUpdateParams
		if err := json.Unmarshal(envelope.Params, &params); err == nil {
			s.mu.Lock()
			if unityConn, ok := s.unityConns[params.ProjectID]; ok {
				unityConn.tools = params.Tools
				unityConn.schemaHash = ComputeSchemaHash(params.Tools)
			}
			s.mu.Unlock()
		}
	case "unity.busy":
		var params unityBusyParams
		if err := json.Unmarshal(envelope.Params, &params); err == nil {
			s.handleUnityBusy(conn, params)
		}
	case "unity.pong":
		return
	case "client.cancelRequest":
		return
	}
}

func (s *Server) handleResponse(conn *serverConnection, envelope rpcEnvelope) {
	requestID := extractResponseID(envelope.ID)
	if requestID == "" {
		return
	}

	s.mu.Lock()
	pending, ok := s.pending[requestID]
	if ok && pending.targetConn != conn {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if !ok {
		return
	}

	if envelope.Error != nil {
		_ = pending.clientConn.send(newErrorResponse(pending.clientRequestID, envelope.Error.Code, envelope.Error.Message, nil))
		s.clearPending(requestID)
		return
	}

	var toolResult unityToolResultEnvelope
	if err := json.Unmarshal(envelope.Result, &toolResult); err != nil {
		_ = pending.clientConn.send(newErrorResponse(pending.clientRequestID, InvalidRequest, "invalid Unity tool response", nil))
		s.clearPending(requestID)
		return
	}

	if toolResult.Pending {
		s.mu.Lock()
		pending.awaitingUnityContinuation = true
		s.mu.Unlock()
		return
	}

	var forwarded any
	if err := json.Unmarshal(envelope.Result, &forwarded); err != nil {
		forwarded = map[string]any{
			"success": toolResult.Success,
			"result":  toolResult.Result,
			"error":   toolResult.Error,
		}
	}
	_ = pending.clientConn.send(newSuccessResponse(pending.clientRequestID, forwarded))
	s.clearPending(requestID)
}

func (s *Server) handleUnityRegister(conn *serverConnection, params unityRegisterParams) {
	conn.kind = "unity"
	conn.projectID = params.ProjectID
	conn.projectName = params.ProjectName
	conn.gitRoot = params.GitRoot
	conn.tools = params.Tools
	conn.schemaHash = ComputeSchemaHash(params.Tools)

	s.mu.Lock()
	if existing := s.unityConns[params.ProjectID]; existing != nil && existing != conn {
		_ = existing.conn.Close()
	}
	s.unityConns[params.ProjectID] = conn
	s.mu.Unlock()

	s.resumePendingRequests(params.ProjectID, params.PendingRequestIDs)
}

func (s *Server) handleClientRegister(conn *serverConnection, params clientRegisterParams) {
	conn.kind = "client"
	conn.clientID = params.ClientID

	s.mu.Lock()
	s.clientConns[params.ClientID] = conn
	s.mu.Unlock()
}

func (s *Server) buildProjectsResult(includeTools bool) ClientListProjectsResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := ClientListProjectsResult{
		Projects: make([]ProjectInfo, 0, len(s.unityConns)),
	}
	for _, unityConn := range s.unityConns {
		project := ProjectInfo{
			ID:         unityConn.projectID,
			Name:       unityConn.projectName,
			GitRoot:    unityConn.gitRoot,
			Connected:  true,
			SchemaHash: unityConn.schemaHash,
		}
		if includeTools {
			project.Tools = append([]ToolDefinition(nil), unityConn.tools...)
		}
		result.Projects = append(result.Projects, project)
	}
	return result
}

func (s *Server) handleClientToolCall(conn *serverConnection, clientRequestID string, params clientToolCallParams) {
	targetConn, code, message := s.resolveToolTarget(params.Tool, params.ProjectID)
	if targetConn == nil {
		_ = conn.send(newErrorResponse(clientRequestID, code, message, nil))
		return
	}

	s.mu.Lock()
	if len(s.pending) >= maxPendingRequests {
		s.mu.Unlock()
		_ = conn.send(newErrorResponse(clientRequestID, ServerOverloaded, "server overloaded", nil))
		return
	}

	daemonRequestID := fmt.Sprintf("d-%d-%d", time.Now().UnixMilli(), atomic.AddUint64(&s.nextRequestID, 1))
	requestTimeout := defaultRequestTimeout
	if params.TimeoutMS > 0 {
		requestTimeout = time.Duration(params.TimeoutMS) * time.Millisecond
	}
	pending := &pendingRequest{
		daemonRequestID: daemonRequestID,
		clientRequestID: clientRequestID,
		clientConn:      conn,
		targetConn:      targetConn,
		targetProjectID: targetConn.projectID,
		tool:            params.Tool,
		args:            params.Args,
		timeout:         requestTimeout,
	}
	pending.timeoutTimer = time.AfterFunc(requestTimeout, func() {
		_ = conn.send(newErrorResponse(clientRequestID, ToolTimeout, "tool execution timed out", nil))
		s.clearPending(daemonRequestID)
	})
	s.pending[daemonRequestID] = pending
	s.mu.Unlock()

	_ = targetConn.send(newRequest(daemonRequestID, "daemon.executeTool", map[string]any{
		"tool": params.Tool,
		"args": params.Args,
	}))
}

func (s *Server) resolveToolTarget(tool string, projectID string) (*serverConnection, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if projectID != "" {
		conn := s.unityConns[projectID]
		if conn == nil {
			return nil, UnityNotConnected, fmt.Sprintf("No Unity project connected with ID: %s", projectID)
		}
		for _, candidate := range conn.tools {
			if candidate.Name == tool {
				return conn, 0, ""
			}
		}
		return nil, ToolNotFound, fmt.Sprintf("Tool not found: %s", tool)
	}

	var matches []*serverConnection
	for _, unityConn := range s.unityConns {
		for _, candidate := range unityConn.tools {
			if candidate.Name == tool {
				matches = append(matches, unityConn)
				break
			}
		}
	}

	switch len(matches) {
	case 0:
		if len(s.unityConns) == 0 {
			return nil, UnityNotConnected, "No Unity project connected"
		}
		return nil, ToolNotFound, fmt.Sprintf("No connected Unity project provides tool %q", tool)
	case 1:
		return matches[0], 0, ""
	default:
		return nil, UnityNotConnected, fmt.Sprintf("Multiple Unity projects provide tool %q; specify project_id", tool)
	}
}

func (s *Server) handleUnityBusy(conn *serverConnection, params unityBusyParams) {
	requestID := fmt.Sprintf("%v", params.RequestID)

	s.mu.Lock()
	pending, ok := s.pending[requestID]
	if !ok || pending.targetConn != conn {
		s.mu.Unlock()
		return
	}
	pending.retryCount++
	if pending.retryCount >= maxBusyRetries {
		s.mu.Unlock()
		_ = pending.clientConn.send(newErrorResponse(pending.clientRequestID, UnityBusy, "Unity remained busy after retries", nil))
		s.clearPending(requestID)
		return
	}

	targetConn := s.unityConns[pending.targetProjectID]
	retryAfter := time.Duration(params.RetryAfterMS) * time.Millisecond
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	if pending.busyRetryTimer != nil {
		pending.busyRetryTimer.Stop()
	}
	pending.busyRetryTimer = time.AfterFunc(retryAfter, func() {
		s.mu.Lock()
		latest := s.pending[requestID]
		target := s.unityConns[pending.targetProjectID]
		if latest == nil {
			s.mu.Unlock()
			return
		}
		if target == nil {
			latest.awaitingReconnection = true
			s.mu.Unlock()
			return
		}
		latest.targetConn = target
		s.mu.Unlock()
		_ = target.send(newRequest(requestID, "daemon.executeTool", map[string]any{
			"tool": latest.tool,
			"args": latest.args,
		}))
	})
	s.mu.Unlock()

	if targetConn == nil {
		return
	}
}

func (s *Server) resumePendingRequests(projectID string, resumedIDs []string) {
	resumed := map[string]struct{}{}
	for _, id := range resumedIDs {
		resumed[id] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	conn := s.unityConns[projectID]
	for _, pending := range s.pending {
		if pending.targetProjectID != projectID || !pending.awaitingReconnection {
			continue
		}

		pending.awaitingReconnection = false
		pending.targetConn = conn
		if pending.awaitingUnityContinuation {
			continue
		}
		if _, ok := resumed[pending.daemonRequestID]; ok {
			pending.awaitingUnityContinuation = true
			continue
		}

		_ = conn.send(newRequest(pending.daemonRequestID, "daemon.executeTool", map[string]any{
			"tool": pending.tool,
			"args": pending.args,
		}))
	}
}

func (s *Server) clearPending(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending := s.pending[requestID]
	if pending == nil {
		return
	}
	if pending.timeoutTimer != nil {
		pending.timeoutTimer.Stop()
	}
	if pending.busyRetryTimer != nil {
		pending.busyRetryTimer.Stop()
	}
	delete(s.pending, requestID)
}

func (s *Server) handleDisconnect(conn *serverConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if conn.kind == "unity" && conn.projectID != "" {
		if current := s.unityConns[conn.projectID]; current == conn {
			delete(s.unityConns, conn.projectID)
			for _, pending := range s.pending {
				if pending.targetProjectID == conn.projectID {
					pending.awaitingReconnection = true
				}
			}
		}
	}

	if conn.kind == "client" && conn.clientID != "" {
		if current := s.clientConns[conn.clientID]; current == conn {
			delete(s.clientConns, conn.clientID)
		}
	}

	_ = conn.conn.Close()
}

func (c *serverConnection) send(payload any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(append(data, '\n'))
	return err
}
