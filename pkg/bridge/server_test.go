package bridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleResponsePendingKeepsOriginalTimeout(t *testing.T) {
	server := NewServer()
	clientConn, daemonConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = daemonConn.Close() }()

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
	unityConnection := &serverConnection{id: "unity-1"}
	pending.targetConn = unityConnection
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

	server.handleResponse(unityConnection, rpcEnvelope{
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

	_ = daemonConn.Close()
	select {
	case <-readDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reader goroutine did not exit")
	}
}

func TestHandleResponseRejectsDifferentUnityConnection(t *testing.T) {
	server := NewServer()
	expected := &serverConnection{id: "unity-expected"}
	server.pending["d-1"] = &pendingRequest{
		daemonRequestID: "d-1",
		targetConn:      expected,
	}

	responseID, err := json.Marshal("d-1")
	if err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(unityToolResultEnvelope{Success: true, Result: "forged"})
	if err != nil {
		t.Fatal(err)
	}
	server.handleResponse(&serverConnection{id: "unity-attacker"}, rpcEnvelope{ID: responseID, Result: result})

	server.mu.Lock()
	_, stillPending := server.pending["d-1"]
	server.mu.Unlock()
	if !stillPending {
		t.Fatal("response from a different Unity connection completed the pending request")
	}
}

func TestHandleUnityBusyRejectsDifferentUnityConnection(t *testing.T) {
	server := NewServer()
	expected := &serverConnection{id: "unity-expected"}
	pending := &pendingRequest{daemonRequestID: "d-1", targetConn: expected}
	server.pending["d-1"] = pending

	server.handleUnityBusy(&serverConnection{id: "unity-attacker"}, unityBusyParams{RequestID: "d-1", RetryAfterMS: 1})

	if pending.retryCount != 0 || pending.busyRetryTimer != nil {
		t.Fatal("busy notification from a different Unity connection changed the pending request")
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

func TestResumePendingRequestsKeepsWaitingWhenConnectionDisappears(t *testing.T) {
	server := NewServer()
	pending := &pendingRequest{
		daemonRequestID:      "d-1",
		targetProjectID:      "project-1",
		awaitingReconnection: true,
	}
	server.pending["d-1"] = pending

	server.resumePendingRequests("project-1", nil)

	if !pending.awaitingReconnection || pending.targetConn != nil {
		t.Fatalf("pending request lost reconnection state: %+v", pending)
	}
}

func TestResumePendingRequestsDoesNotHoldServerLockWhileSending(t *testing.T) {
	server := NewServer()
	serverSide, peer := net.Pipe()
	defer func() { _ = peer.Close() }()
	target := &serverConnection{id: "unity-1", conn: serverSide, projectID: "project-1"}
	server.unityConns["project-1"] = target
	server.pending["d-1"] = &pendingRequest{
		daemonRequestID:      "d-1",
		targetProjectID:      "project-1",
		tool:                 "test-tool",
		awaitingReconnection: true,
	}

	resumeDone := make(chan struct{})
	go func() {
		server.resumePendingRequests("project-1", nil)
		close(resumeDone)
	}()
	time.Sleep(20 * time.Millisecond)

	lockAcquired := make(chan int, 1)
	go func() {
		server.mu.Lock()
		pendingCount := len(server.pending)
		server.mu.Unlock()
		lockAcquired <- pendingCount
	}()
	select {
	case pendingCount := <-lockAcquired:
		if pendingCount != 1 {
			t.Fatalf("pending count = %d, want 1", pendingCount)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("server lock was held by a blocked connection write")
	}

	_ = peer.Close()
	select {
	case <-resumeDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("resume did not return after connection closed")
	}
}

func TestStopClosesAcceptedUnregisteredConnection(t *testing.T) {
	server := NewServer()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()

	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	deadline := time.Now().Add(time.Second)
	for {
		server.mu.Lock()
		count := len(server.connections)
		server.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("accepted connection was not tracked")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("unregistered connection remained open after Stop")
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Serve did not return after Stop")
	}
}

func TestServeAfterStopReturnsImmediately(t *testing.T) {
	server := NewServer()
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(listener); err != nil {
		t.Fatal(err)
	}
}

func TestClaimPendingAllowsOnlyOneCompletion(t *testing.T) {
	server := NewServer()
	server.pending["d-1"] = &pendingRequest{daemonRequestID: "d-1"}
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if server.claimPending("d-1", nil) != nil {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := claimed.Load(); got != 1 {
		t.Fatalf("pending request claimed %d times, want 1", got)
	}
}

func TestClientDisconnectCancelsOwnedPendingRequests(t *testing.T) {
	server := NewServer()
	serverSide, peer := net.Pipe()
	defer func() { _ = peer.Close() }()
	client := &serverConnection{kind: "client", clientID: "client-1", conn: serverSide}
	server.clientConns[client.clientID] = client
	server.connections[client] = struct{}{}
	server.pending["owned"] = &pendingRequest{clientConn: client, timeoutTimer: time.NewTimer(time.Hour)}
	server.pending["other"] = &pendingRequest{clientConn: &serverConnection{id: "other"}}

	server.handleDisconnect(client)

	server.mu.Lock()
	_, ownedExists := server.pending["owned"]
	_, otherExists := server.pending["other"]
	server.mu.Unlock()
	if ownedExists || !otherExists {
		t.Fatalf("pending after disconnect: owned=%v other=%v", ownedExists, otherExists)
	}
}

func TestUnityReregisterAndProjectReadsAreRaceFree(t *testing.T) {
	server := NewServer()
	conn := &serverConnection{id: "unity-1"}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(index int) {
			defer wg.Done()
			server.handleUnityRegister(conn, unityRegisterParams{
				ProjectID: "project-1",
				Tools:     []ToolDefinition{{Name: fmt.Sprintf("tool-%d", index)}},
			})
		}(i)
		go func() {
			defer wg.Done()
			_ = server.buildProjectsResult(true)
		}()
	}
	wg.Wait()
}

func TestUnityRegisterPublishesConsoleLogPath(t *testing.T) {
	server := NewServer()
	conn := &serverConnection{id: "unity-1"}
	server.handleUnityRegister(conn, unityRegisterParams{
		ProjectID:      "/repos/game",
		ProjectName:    "Game",
		GitRoot:        "/repos/game",
		ConsoleLogPath: "/logs/game.log",
	})

	result := server.buildProjectsResult(false)
	if len(result.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(result.Projects))
	}
	if got := result.Projects[0].ConsoleLogPath; got != "/logs/game.log" {
		t.Fatalf("consoleLogPath = %q, want %q", got, "/logs/game.log")
	}
}

func TestUnityRegisterPublishesPackageVersion(t *testing.T) {
	server := NewServer()
	conn := &serverConnection{id: "unity-1"}
	server.handleUnityRegister(conn, unityRegisterParams{
		ProjectID:      "/repos/game",
		ProjectName:    "Game",
		PackageVersion: "0.11.0",
	})

	result := server.buildProjectsResult(false)
	if len(result.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(result.Projects))
	}
	if got := result.Projects[0].PackageVersion; got != "0.11.0" {
		t.Fatalf("packageVersion = %q, want %q", got, "0.11.0")
	}
}

func TestUnityRegisterWithoutPackageVersionOmitsItFromProjects(t *testing.T) {
	server := NewServer()
	conn := &serverConnection{id: "unity-1"}
	server.handleUnityRegister(conn, unityRegisterParams{
		ProjectID:   "/repos/game",
		ProjectName: "Game",
	})

	result := server.buildProjectsResult(false)
	if len(result.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(result.Projects))
	}
	if got := result.Projects[0].PackageVersion; got != "" {
		t.Fatalf("packageVersion = %q, want empty", got)
	}

	encoded, err := json.Marshal(result.Projects[0])
	if err != nil {
		t.Fatalf("marshal project: %v", err)
	}
	var project map[string]any
	if err := json.Unmarshal(encoded, &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	if _, exists := project["packageVersion"]; exists {
		t.Fatalf("packageVersion should be omitted, got %s", encoded)
	}
}

func TestUnityRegisterResponseIncludesLatestPackageVersion(t *testing.T) {
	providerCalls := 0
	server := NewServer(WithLatestUnityPackageVersionProvider(func() string {
		providerCalls++
		return "0.12.0"
	}))

	result := invokeUnityRegister(t, server, unityRegisterParams{
		ProjectID:      "/repos/game",
		ProjectName:    "Game",
		PackageVersion: "0.11.0",
	})

	if got := result["success"]; got != true {
		t.Fatalf("success = %#v, want true", got)
	}
	if got := result["latestPackageVersion"]; got != "0.12.0" {
		t.Fatalf("latestPackageVersion = %#v, want %q", got, "0.12.0")
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls)
	}
	assertMinPackageVersion(t, result)
}

func TestUnityRegisterResponseOmitsUnknownPackageVersions(t *testing.T) {
	tests := []struct {
		name   string
		server *Server
	}{
		{
			name:   "nil provider",
			server: NewServer(WithLatestUnityPackageVersionProvider(nil)),
		},
		{
			name: "empty provider",
			server: NewServer(WithLatestUnityPackageVersionProvider(func() string {
				return ""
			})),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := invokeUnityRegister(t, tt.server, unityRegisterParams{
				ProjectID:   "/repos/game",
				ProjectName: "Game",
			})

			if got := result["success"]; got != true {
				t.Fatalf("success = %#v, want true", got)
			}
			if _, exists := result["latestPackageVersion"]; exists {
				t.Fatalf("latestPackageVersion should be omitted, got %#v", result)
			}
			assertMinPackageVersion(t, result)
		})
	}
}

func invokeUnityRegister(t *testing.T, server *Server, params unityRegisterParams) map[string]any {
	t.Helper()

	serverSide, unitySide := net.Pipe()
	defer func() { _ = serverSide.Close() }()
	defer func() { _ = unitySide.Close() }()

	encodedParams, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal unity.register params: %v", err)
	}

	done := make(chan struct{})
	go func() {
		server.handleRequest(&serverConnection{id: "unity-test", conn: serverSide}, "register-1", rpcEnvelope{
			Method: "unity.register",
			Params: encodedParams,
		})
		close(done)
	}()

	if err := unitySide.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set response deadline: %v", err)
	}
	responseLine, err := bufio.NewReader(unitySide).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read unity.register response: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("unity.register handler did not return")
	}

	var response struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(responseLine, &response); err != nil {
		t.Fatalf("decode unity.register response: %v", err)
	}
	if response.Result == nil {
		t.Fatalf("unity.register response has no result: %s", responseLine)
	}
	return response.Result
}

func assertMinPackageVersion(t *testing.T, result map[string]any) {
	t.Helper()

	if MinRecommendedUnityPackageVersion == "" {
		if _, exists := result["minPackageVersion"]; exists {
			t.Fatalf("minPackageVersion should be omitted, got %#v", result)
		}
		return
	}

	if got := result["minPackageVersion"]; got != MinRecommendedUnityPackageVersion {
		t.Fatalf("minPackageVersion = %#v, want %q", got, MinRecommendedUnityPackageVersion)
	}
}
