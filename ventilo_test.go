package main

import (
	"bufio"
	"encoding/json"
    "fmt"
    "io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestChannelsEndpoint(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Add some test channels
	server.listeners["test1"] = []chan string{make(chan string)}
	server.listeners["test2"] = []chan string{make(chan string), make(chan string)}
	server.messages["test1"] = 5
	server.messages["test2"] = 10

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Test the channels endpoint
	resp, err := http.Get(ts.URL + channels)
	if err != nil {
		t.Fatalf("Failed to get channels: %v", err)
	}
	defer resp.Body.Close()

	// Check the response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	// Decode the JSON response
	var topics []Topic
	if err := json.NewDecoder(resp.Body).Decode(&topics); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	// Check that we got the expected number of topics
	if len(topics) != 2 {
		t.Errorf("Expected 2 topics, got %d", len(topics))
	}

	// Check the contents of the topics
	topicMap := make(map[string]Topic)
	for _, topic := range topics {
		topicMap[topic.Name] = topic
	}

	test1, exists := topicMap["test1"]
	if !exists {
		t.Error("Expected to find topic 'test1'")
	} else {
		if test1.Listeners != 1 {
			t.Errorf("Expected 1 listener for test1, got %d", test1.Listeners)
		}
		if test1.Messages != 5 {
			t.Errorf("Expected 5 messages for test1, got %d", test1.Messages)
		}
	}

	test2, exists := topicMap["test2"]
	if !exists {
		t.Error("Expected to find topic 'test2'")
	} else {
		if test2.Listeners != 2 {
			t.Errorf("Expected 2 listeners for test2, got %d", test2.Listeners)
		}
		if test2.Messages != 10 {
			t.Errorf("Expected 10 messages for test2, got %d", test2.Messages)
		}
	}
}

func TestBroadcastEndpoint(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Create a channel to receive messages
	msgChan := make(chan string, 1)

	// Listen to the test channel
	server.mutex.Lock()
	server.listeners["testChannel"] = append(server.listeners["testChannel"], msgChan)
	server.mutex.Unlock()

	// Broadcast a message
	form := url.Values{}
	form.Add("message", "Hello, world!")
	resp, err := http.PostForm(ts.URL+broadcast+"testChannel", form)
	if err != nil {
		t.Fatalf("Failed to broadcast message: %v", err)
	}
	defer resp.Body.Close()

	// Check the response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	// Check that the message was received
	select {
	case msg := <-msgChan:
		if msg != "Hello, world!" {
			t.Errorf("Expected message 'Hello, world!', got '%s'", msg)
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for message")
	}

	// Check that the message count was incremented
	server.mutex.Lock()
	if server.messages["testChannel"] != 1 {
		t.Errorf("Expected message count 1, got %d", server.messages["testChannel"])
	}
	server.mutex.Unlock()
}

func TestWebSocketEndpoint(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect to the WebSocket endpoint
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + listen + "testChannel"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Check that the channel was created
	server.mutex.Lock()
	if len(server.listeners["testChannel"]) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(server.listeners["testChannel"]))
	}
	server.mutex.Unlock()

	// Broadcast a message
	form := url.Values{}
	form.Add("message", "Hello, WebSocket!")
	resp, err := http.PostForm(ts.URL+broadcast+"testChannel", form)
	if err != nil {
		t.Fatalf("Failed to broadcast message: %v", err)
	}
	resp.Body.Close()

	// Check that the message was received
	msgType, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Errorf("Expected text message, got %d", msgType)
	}
	if string(msg) != "Hello, WebSocket!" {
		t.Errorf("Expected message 'Hello, WebSocket!', got '%s'", string(msg))
	}
}

// MockSSEClient simulates an SSE client
type MockSSEClient struct {
	resp   *http.Response
	reader *bufio.Reader
	t      *testing.T
}

func NewMockSSEClient(t *testing.T, url string) (*MockSSEClient, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return &MockSSEClient{
		resp:   resp,
		reader: bufio.NewReader(resp.Body),
		t:      t,
	}, nil
}

func (c *MockSSEClient) ReadEvent() (string, error) {
	var event string
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return "", fmt.Errorf("connection closed")
			}
			return "", err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line signals the end of an event
			break
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			event = data
		}
	}

	return event, nil
}

func (c *MockSSEClient) Close() {
	c.resp.Body.Close()
}

func TestSSEEndpoint(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect to the SSE endpoint
	sseClient, err := NewMockSSEClient(t, ts.URL+listen+"testChannel")
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer sseClient.Close()

	// Check that the channel was created
	server.mutex.Lock()
	if len(server.listeners["testChannel"]) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(server.listeners["testChannel"]))
	}
	server.mutex.Unlock()

	// Broadcast a message
	form := url.Values{}
	form.Add("message", "Hello, SSE!")
	resp, err := http.PostForm(ts.URL+broadcast+"testChannel", form)
	if err != nil {
		t.Fatalf("Failed to broadcast message: %v", err)
	}
	resp.Body.Close()

	// Check that the message was received
	event, err := sseClient.ReadEvent()
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}
	if event != "Hello, SSE!" {
		t.Errorf("Expected message 'Hello, SSE!', got '%s'", event)
	}
}

func TestMultipleListeners(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect to the WebSocket endpoint
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + listen + "sharedChannel"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Connect to the SSE endpoint
	sseClient, err := NewMockSSEClient(t, ts.URL+listen+"sharedChannel")
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer sseClient.Close()

	// Check that the channel has two listeners
	server.mutex.Lock()
	if len(server.listeners["sharedChannel"]) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(server.listeners["sharedChannel"]))
	}
	server.mutex.Unlock()

	// Broadcast a message
	form := url.Values{}
	form.Add("message", "Hello, everyone!")
	resp, err := http.PostForm(ts.URL+broadcast+"sharedChannel", form)
	if err != nil {
		t.Fatalf("Failed to broadcast message: %v", err)
	}
	resp.Body.Close()

	// Check that the WebSocket client received the message
	msgType, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Errorf("Expected text message, got %d", msgType)
	}
	if string(msg) != "Hello, everyone!" {
		t.Errorf("Expected message 'Hello, everyone!', got '%s'", string(msg))
	}

	// Check that the SSE client received the message
	event, err := sseClient.ReadEvent()
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}
	if event != "Hello, everyone!" {
		t.Errorf("Expected message 'Hello, everyone!', got '%s'", event)
	}
}

func TestHangup(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test channel
	channel := make(chan string, 1)
	server.listeners["testChannel"] = append(server.listeners["testChannel"], channel)

	// Verify the channel exists
	server.mutex.Lock()
	if len(server.listeners["testChannel"]) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(server.listeners["testChannel"]))
	}
	server.mutex.Unlock()

	// Hangup the channel
	server.hangup("testChannel", channel)

	// Verify the channel was removed
	server.mutex.Lock()
	if len(server.listeners["testChannel"]) != 0 {
		t.Errorf("Expected 0 listeners, got %d", len(server.listeners["testChannel"]))
	}
	server.mutex.Unlock()

	// Send a message to the channel (should be drained)
	go func() {
		channel <- "Test message"
	}()

	// Wait for the message to be drained
	time.Sleep(100 * time.Millisecond)
}

func TestNotFoundEndpoint(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Test an invalid endpoint
	resp, err := http.Get(ts.URL + "/invalid/endpoint")
	if err != nil {
		t.Fatalf("Failed to get invalid endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Check that we got a 404
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status NotFound, got %v", resp.Status)
	}
}

func TestConcurrentBroadcast(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Create multiple listeners
	const numListeners = 5
	wsClients := make([]*websocket.Conn, numListeners)

	for i := 0; i < numListeners; i++ {
		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + listen + "concurrentChannel"
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer ws.Close()
		wsClients[i] = ws
	}

	// Check that the channel has the expected number of listeners
	server.mutex.Lock()
	if len(server.listeners["concurrentChannel"]) != numListeners {
		t.Errorf("Expected %d listeners, got %d", numListeners, len(server.listeners["concurrentChannel"]))
	}
	server.mutex.Unlock()

	// Broadcast a message
	form := url.Values{}
	form.Add("message", "Concurrent broadcast test")
	resp, err := http.PostForm(ts.URL+broadcast+"concurrentChannel", form)
	if err != nil {
		t.Fatalf("Failed to broadcast message: %v", err)
	}
	resp.Body.Close()

	// Check that all clients received the message
	for i, ws := range wsClients {
		msgType, msg, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("Client %d failed to read message: %v", i, err)
		}
		if msgType != websocket.TextMessage {
			t.Errorf("Client %d expected text message, got %d", i, msgType)
		}
		if string(msg) != "Concurrent broadcast test" {
			t.Errorf("Client %d expected 'Concurrent broadcast test', got '%s'", i, string(msg))
		}
	}
}

func TestWebSocketDisconnect(t *testing.T) {
	// Create a new server instance
	server := newServer()

	// Create a test HTTP server
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect to the WebSocket endpoint
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + listen + "disconnectChannel"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	// Verify the channel exists
	server.mutex.Lock()
	initialListeners := len(server.listeners["disconnectChannel"])
	if initialListeners != 1 {
		t.Errorf("Expected 1 listener, got %d", initialListeners)
	}
	server.mutex.Unlock()

	// Close the WebSocket connection
	ws.Close()

	// Give some time for the server to process the disconnection
	time.Sleep(100 * time.Millisecond)

	// Verify the channel was removed
	server.mutex.Lock()
	finalListeners := len(server.listeners["disconnectChannel"])
	if finalListeners != 0 {
		t.Errorf("Expected 0 listeners after disconnect, got %d", finalListeners)
	}
	server.mutex.Unlock()
}
