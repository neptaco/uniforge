package bridge

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestHandleResponsePendingKeepsOriginalTimeout(t *testing.T) {
	server := NewServer()
	clientConn, daemonConn := net.Pipe()
	defer clientConn.Close()
	defer daemonConn.Close()

	clientMessages := make(chan map[string]any, 4)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(clientConn)
		for scanner.Scan() {
			var payload map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &payload); err == nil {
				clientMessages <- payload
			}
		}
	}()

	requestID := "d-1"
	clientRequestID := "go-1"
	requestTimeout := 80 * time.Millisecond

	pending := &pendingRequest{
		daemonRequestID: requestID,
		clientRequestID: clientRequestID,
		clientConn: &serverConnection{
			id:   "client-1",
			conn: daemonConn,
		},
		timeout: requestTimeout,
	}
	pending.timeoutTimer = time.AfterFunc(requestTimeout, func() {
		_ = pending.clientConn.send(newErrorResponse(clientRequestID, ToolTimeout, "tool execution timed out", nil))
		server.clearPending(requestID)
	})

	server.pending[requestID] = pending

	time.Sleep(30 * time.Millisecond)

	responseID, err := json.Marshal(requestID)
	if err != nil {
		t.Fatalf("marshal response id: %v", err)
	}

	result, err := json.Marshal(unityToolResultEnvelope{Success: true, Pending: true})
	if err != nil {
		t.Fatalf("marshal pending result: %v", err)
	}

	server.handleResponse(&serverConnection{id: "unity-1"}, rpcEnvelope{
		ID:     responseID,
		Result: result,
	})

	time.Sleep(70 * time.Millisecond)

	server.mu.Lock()
	_, stillPending := server.pending[requestID]
	server.mu.Unlock()
	if stillPending {
		t.Fatal("pending request should time out at the original deadline")
	}

	select {
	case message := <-clientMessages:
		errorPayload, ok := message["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected error response, got %#v", message)
		}
		if got := errorPayload["message"]; got != "tool execution timed out" {
			t.Fatalf("timeout message = %v, want %q", got, "tool execution timed out")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for timeout response")
	}

	daemonConn.Close()
	select {
	case <-readDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reader goroutine did not exit")
	}
}

func TestHandleDisconnectDoesNotRemoveReplacementUnityConnection(t *testing.T) {
	server := NewServer()
	oldConn, oldPeer := net.Pipe()
	defer func() { _ = oldPeer.Close() }()
	newConn, newPeer := net.Pipe()
	defer func() { _ = newConn.Close() }()
	defer func() { _ = newPeer.Close() }()

	oldConnection := &serverConnection{
		kind:      "unity",
		projectID: "project-1",
		conn:      oldConn,
	}
	replacement := &serverConnection{
		kind:      "unity",
		projectID: "project-1",
		conn:      newConn,
	}
	pending := &pendingRequest{targetProjectID: "project-1"}
	server.unityConns["project-1"] = replacement
	server.pending["request-1"] = pending

	server.handleDisconnect(oldConnection)

	if got := server.unityConns["project-1"]; got != replacement {
		t.Fatal("replacement Unity connection was removed by the old connection's disconnect")
	}
	if pending.awaitingReconnection {
		t.Fatal("pending request was marked disconnected while the replacement is connected")
	}
}

func TestHandleDisconnectDoesNotRemoveReplacementClientConnection(t *testing.T) {
	server := NewServer()
	oldConn, oldPeer := net.Pipe()
	defer func() { _ = oldPeer.Close() }()
	newConn, newPeer := net.Pipe()
	defer func() { _ = newConn.Close() }()
	defer func() { _ = newPeer.Close() }()

	oldConnection := &serverConnection{
		kind:     "client",
		clientID: "client-1",
		conn:     oldConn,
	}
	replacement := &serverConnection{
		kind:     "client",
		clientID: "client-1",
		conn:     newConn,
	}
	server.clientConns["client-1"] = replacement

	server.handleDisconnect(oldConnection)

	if got := server.clientConns["client-1"]; got != replacement {
		t.Fatal("replacement client connection was removed by the old connection's disconnect")
	}
}
