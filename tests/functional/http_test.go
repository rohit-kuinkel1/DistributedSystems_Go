package functional

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// TestHTTPServerAndClient tests the HTTP server and client implementation
func TestHTTPServerAndClient(t *testing.T) {
	server := http.ServerFactory("localhost", 8081)

	server.RegisterHandler(
		http.POST,
		"/test",
		func(req *http.Request) *http.Response {
			var data types.SensorData
			err := json.Unmarshal(req.Body, &data)
			if err != nil {
				return http.CreateTextResponse(http.StatusBadRequest, []byte(fmt.Sprintf("Error: %v", err)))
			}

			//return the data as JSON response
			responseData, _ := json.Marshal(data)
			return http.CreateJSONResponse(http.StatusOK, responseData)
		},
	)

	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	//wait for server to start
	time.Sleep(100 * time.Millisecond)

	client := http.HttpClientFactory(5 * time.Second)
	testData := types.SensorData{
		SensorID:  "test-sensor-1",
		Timestamp: time.Now(),
		Value:     23.5,
		Unit:      "°C",
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	//send POST request with the json data
	resp, err := client.PostJSON("http://localhost:8081/test", jsonData)
	if err != nil {
		t.Fatalf("Failed to send POST request: %v", err)
	}

	//check the response status for a 200
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	//parse response body
	var responseData types.SensorData
	err = json.Unmarshal(resp.Body, &responseData)
	if err != nil {
		t.Errorf("Failed to parse response JSON: %v", err)
	}

	//compare request and response data
	if responseData.SensorID != testData.SensorID {
		t.Errorf("Expected sensor ID %s, got %s", testData.SensorID, responseData.SensorID)
	}

	if responseData.Value != testData.Value {
		t.Errorf("Expected value %.1f, got %.1f", testData.Value, responseData.Value)
	}

	if responseData.Unit != testData.Unit {
		t.Errorf("Expected unit %s, got %s", testData.Unit, responseData.Unit)
	}

	log.Println("HTTP server and client test passed successfully")
}

// TestHTTPRequestParsing tests the HTTP request parsing functionality
func TestHTTPRequestParsing(t *testing.T) {
	requestStr := "POST /data HTTP/1.1\r\n" +
		"Host: localhost:8080\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 10\r\n" +
		"\r\n" +
		"0123456789"

	mockConn := MockConnFactory([]byte(requestStr))

	//parse the request
	req, err := http.ParseRequest(mockConn)
	if err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("Expected method POST, got %s", req.Method)
	}

	if req.Path != "/data" {
		t.Errorf("Expected path /data, got %s", req.Path)
	}

	if req.Version != "HTTP/1.1" {
		t.Errorf("Expected version HTTP/1.1, got %s", req.Version)
	}

	host, ok := req.Headers["Host"]
	if !ok {
		t.Error("Host header not found")
	} else if host != "localhost:8080" {
		t.Errorf("Expected host localhost:8080, got %s", host)
	}

	if req.ContentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", req.ContentType)
	}

	if req.ContentLen != 10 {
		t.Errorf("Expected content length 10, got %d", req.ContentLen)
	}

	if string(req.Body) != "0123456789" {
		t.Errorf("Expected body 0123456789, got %s", string(req.Body))
	}

	log.Println("HTTP request parsing test passed successfully")
}

// MockConn is a mock implementation of net.Conn for testing
type MockConn struct {
	readData []byte
	readPos  int
	written  []byte
}

// MockConnFactory creates a new mock connection with the given read data
func MockConnFactory(readData []byte) *MockConn {
	return &MockConn{
		readData: readData,
		written:  make([]byte, 0),
	}
}

// Read reads data from the mock connection
func (m *MockConn) Read(b []byte) (n int, err error) {
	if m.readPos >= len(m.readData) {
		return 0, fmt.Errorf("end of data")
	}

	n = copy(b, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

// Write writes data to the mock connection
func (m *MockConn) Write(b []byte) (n int, err error) {
	m.written = append(m.written, b...)
	return len(b), nil
}

// Close closes the mock connection
func (m *MockConn) Close() error {
	return nil
}

// LocalAddr returns the local network address
func (m *MockConn) LocalAddr() net.Addr {
	return &mockAddr{}
}

// RemoteAddr returns the remote network address
func (m *MockConn) RemoteAddr() net.Addr {
	return &mockAddr{}
}

// SetDeadline sets the read and write deadlines
func (m *MockConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets the read deadline
func (m *MockConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline sets the write deadline
func (m *MockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// mockAddr is a mock implementation of net.Addr
type mockAddr struct{}

// Network returns the network type
func (a *mockAddr) Network() string {
	return "tcp"
}

// String returns the string representation
func (a *mockAddr) String() string {
	return "127.0.0.1:12345"
}
