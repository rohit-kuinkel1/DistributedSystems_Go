package http

import (
	"bytes"
	"fmt"
	"net"
	"time"
)

// Response represents an HTTP response
type Response struct {
	StatusCode    int
	StatusText    string
	Headers       map[string]string
	Body          []byte
	ContentType   string
	ContentLength int
}

// Common HTTP status texts
var statusTexts = map[int]string{
	StatusOK:          "OK",
	StatusBadRequest:  "Bad Request",
	StatusNotFound:    "Not Found",
	StatusServerError: "Internal Server Error",
}

// NewResponse creates a new response with default headers
func NewResponse(statusCode int) *Response {
	statusText, ok := statusTexts[statusCode]
	if !ok {
		statusText = "Unknown"
	}

	return &Response{
		StatusCode:  statusCode,
		StatusText:  statusText,
		Headers:     make(map[string]string),
		ContentType: "text/plain",
	}
}

// SetContentType sets the content type and adds the Content-Type header
func (r *Response) SetContentType(contentType string) {
	r.ContentType = contentType
	r.Headers["Content-Type"] = contentType
}

// SetBody sets the response body and updates Content-Length
func (r *Response) SetBody(body []byte) {
	r.Body = body
	r.ContentLength = len(body)
	r.Headers["Content-Length"] = fmt.Sprintf("%d", r.ContentLength)
}

// SetBodyString sets the response body from a string
func (r *Response) SetBodyString(body string) {
	r.SetBody([]byte(body))
}

// SetHeader sets a header value
func (r *Response) SetHeader(key, value string) {
	r.Headers[key] = value
}

// Write sends the response to the connection
func (r *Response) Write(conn net.Conn) error {
	var buf bytes.Buffer

	//write status line
	buf.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.StatusCode, r.StatusText))

	//add server and date headers if not present
	if _, ok := r.Headers["Server"]; !ok {
		r.Headers["Server"] = "IoT-Server/1.0"
	}
	if _, ok := r.Headers["Date"]; !ok {
		r.Headers["Date"] = time.Now().UTC().Format(time.RFC1123)
	}

	//write headers
	for key, value := range r.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	buf.WriteString("\r\n")

	//write body if present
	if r.Body != nil && len(r.Body) > 0 {
		buf.Write(r.Body)
	}

	_, err := conn.Write(buf.Bytes())
	return err
}

// String returns a string representation of the response (for logging)
func (r *Response) String() string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.StatusCode, r.StatusText))

	for key, value := range r.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}

	buf.WriteString("\r\n")

	if r.Body != nil && len(r.Body) > 0 {
		buf.Write(r.Body)
	}

	return buf.String()
}

// CreateJSONResponse creates a response with JSON content type and body
func CreateJSONResponse(statusCode int, body []byte) *Response {
	response := NewResponse(statusCode)
	response.SetContentType("application/json")
	response.SetBody(body)
	return response
}

// CreateHTMLResponse creates a response with HTML content type and body
func CreateHTMLResponse(statusCode int, body []byte) *Response {
	response := NewResponse(statusCode)
	response.SetContentType("text/html")
	response.SetBody(body)
	return response
}

// CreateTextResponse creates a response with plain text content type and body
func CreateTextResponse(statusCode int, body []byte) *Response {
	response := NewResponse(statusCode)
	response.SetContentType("text/plain")
	response.SetBody(body)
	return response
}
