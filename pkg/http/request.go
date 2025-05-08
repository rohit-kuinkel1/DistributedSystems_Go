package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

// as defined in the question, we need to support GET and POST requests for both the server and the sender
const (
	GET  = "GET"
	POST = "POST"
)

// define HTTP status codes that match the widely recognized status codes
const (
	StatusOK          = 200
	StatusBadRequest  = 400
	StatusForbidden   = 401
	StatusNotFound    = 404
	StatusServerError = 500
)

// Request represents a typical HTTP request
type Request struct {
	Method      string
	Path        string
	Version     string
	Headers     map[string]string
	Body        []byte
	ContentType string
	ContentLen  int
}

// ParseRequest parses an HTTP request from a connection
func ParseRequest(conn net.Conn) (*Request, error) {
	reader := bufio.NewReader(conn)
	req := &Request{
		Headers: make(map[string]string),
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("error reading request line: %w", err)
	}

	log.Printf("Request line: %s", line)

	//parse the request line (Method, Path, Version)
	parts := strings.Split(strings.TrimSpace(line), " ")
	if len(parts) != 3 {
		return nil, errors.New("invalid request line format")
	}
	req.Method = parts[0]
	req.Path = parts[1]
	req.Version = parts[2]

	//read the headers now
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("error reading header: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			//an empty line indicates end of headers
			break
		}

		//split header by first colon
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			return nil, errors.New("invalid header format")
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		req.Headers[key] = value

		//check for important headers
		keyLower := strings.ToLower(key)
		if keyLower == "content-type" {
			req.ContentType = value
		} else if keyLower == "content-length" {
			contentLen, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			req.ContentLen = contentLen
		}
	}

	//read body if Content-Length is set and method is POST
	if req.Method == POST && req.ContentLen > 0 {
		body := make([]byte, req.ContentLen)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			return nil, fmt.Errorf("error reading request body: %w", err)
		}
		req.Body = body
		log.Printf("Read request body of length %d", len(req.Body))
	}

	return req, nil
}

// ReadBodyFrom reads the request body from a reader (used for testing)
func (r *Request) ReadBodyFrom(reader io.Reader) error {
	if r.ContentLen <= 0 {
		return nil //no body there to read
	}

	body := make([]byte, r.ContentLen)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}
	r.Body = body
	return nil
}

// String returns a string representation of the request (for logging purposes just like .ToString() in c#)
func (r *Request) String() string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("%s %s %s\r\n", r.Method, r.Path, r.Version))

	for key, value := range r.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}

	buf.WriteString("\r\n")

	if len(r.Body) > 0 {
		buf.Write(r.Body)
	}

	return buf.String()
}
