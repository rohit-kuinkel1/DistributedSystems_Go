package http

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

// HttpClient represents an HTTP client
type HttpClient struct {
	Timeout time.Duration
}

// NewClient creates a new HTTP client with the specified timeout
func HttpClientFactory(timeout time.Duration) *HttpClient {
	return &HttpClient{
		Timeout: timeout,
	}
}

// Get sends an HTTP GET request to the specified URL
func (c *HttpClient) Get(url string) (*Response, error) {
	return c.sendRequest(GET, url, nil, "")
}

// Post sends an HTTP POST request with the specified body and content type
func (c *HttpClient) Post(url string, body []byte, contentType string) (*Response, error) {
	return c.sendRequest(POST, url, body, contentType)
}

// PostJSON is a convenience method for sending JSON data
func (c *HttpClient) PostJSON(url string, jsonData []byte) (*Response, error) {
	return c.Post(url, jsonData, "application/json")
}

// sendRequest sends an HTTP request with the specified method, URL, body, and content type
func (c *HttpClient) sendRequest(method, url string, body []byte, contentType string) (*Response, error) {
	host, port, path, err := parseURL(url)
	if err != nil {
		return nil, err
	}

	//connect to our server
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, c.Timeout)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s: %w", addr, err)
	}
	defer conn.Close()

	//set connection timeout
	err = conn.SetDeadline(time.Now().Add(c.Timeout))
	if err != nil {
		return nil, fmt.Errorf("error setting connection deadline: %w", err)
	}

	var reqBuf bytes.Buffer
	reqBuf.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", method, path))
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", host))

	if body != nil && len(body) > 0 {
		reqBuf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
		reqBuf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	}

	//additional headers
	reqBuf.WriteString("Connection: close\r\n")
	reqBuf.WriteString("\r\n")

	//add the body if present
	if body != nil && len(body) > 0 {
		reqBuf.Write(body)
	}

	start := time.Now() //for RTT measurement
	_, err = conn.Write(reqBuf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	rawResponse, err := io.ReadAll(conn)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	//calc RTT
	rtt := time.Since(start)
	log.Printf("Request completed in %v", rtt)

	resp, err := parseResponse(rawResponse)
	if err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return resp, nil
}

// parseURL extracts host, port, and path from a URL
func parseURL(url string) (host string, port int, path string, err error) {
	port = 80

	if !strings.HasPrefix(url, "http://") {
		url = "http://" + url
	}

	url = strings.TrimPrefix(url, "http://")

	//split into host+port and path
	parts := strings.SplitN(url, "/", 2)
	hostPort := parts[0]

	if len(parts) > 1 {
		path = "/" + parts[1]
	} else {
		path = "/"
	}

	//check if host contains a port
	hostParts := strings.SplitN(hostPort, ":", 2)
	host = hostParts[0]

	if len(hostParts) > 1 {
		port, err = parsePort(hostParts[1])
		if err != nil {
			return "", 0, "", err
		}
	}

	return host, port, path, nil
}

// parsePort converts a port string to an integer
func parsePort(portStr string) (int, error) {
	var port int
	_, err := fmt.Sscanf(portStr, "%d", &port)
	if err != nil {
		return 0, fmt.Errorf("invalid port: %s", portStr)
	}
	return port, nil
}

// parseResponse parses a raw HTTP response
func parseResponse(rawResponse []byte) (*Response, error) {
	//split into header and body
	parts := bytes.SplitN(rawResponse, []byte("\r\n\r\n"), 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid response format")
	}

	headerBytes := parts[0]
	body := parts[1]

	//parse the headers
	headerLines := bytes.Split(headerBytes, []byte("\r\n"))
	if len(headerLines) == 0 {
		return nil, fmt.Errorf("invalid response headers")
	}

	//now parse the status line
	statusLine := string(headerLines[0])
	statusParts := strings.SplitN(statusLine, " ", 3)
	if len(statusParts) < 3 {
		return nil, fmt.Errorf("invalid status line: %s", statusLine)
	}

	//extract status code and text
	statusCode := 0
	_, err := fmt.Sscanf(statusParts[1], "%d", &statusCode)
	if err != nil {
		return nil, fmt.Errorf("invalid status code: %s", statusParts[1])
	}
	statusText := statusParts[2]

	//create response
	resp := &Response{
		StatusCode: statusCode,
		StatusText: statusText,
		Headers:    make(map[string]string),
		Body:       body,
	}

	//now parse the remaining headers
	for i := 1; i < len(headerLines); i++ {
		line := string(headerLines[i])
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		resp.Headers[key] = value

		//check for special headers
		keyLower := strings.ToLower(key)
		if keyLower == "content-type" {
			resp.ContentType = value
		} else if keyLower == "content-length" {
			contentLen := 0
			_, err := fmt.Sscanf(value, "%d", &contentLen)
			if err == nil {
				resp.ContentLength = contentLen
			}
		}
	}

	return resp, nil
}
