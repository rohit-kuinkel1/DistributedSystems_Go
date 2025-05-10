package http

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// RequestHandler defines a function that handles HTTP requests
type RequestHandler func(*Request) *Response

// Server represents an HTTP server
type Server struct {
	Host     string                    //URL for the server to be hosted at; like http://localhost
	Port     int                       //the PORT for the server to be hosted at; 8080 for example
	Handlers map[string]RequestHandler //all the handlers that are supported by this server, for example POST or GET
	listener net.Listener
	wg       sync.WaitGroup
	running  bool
	mutex    sync.Mutex
}

// ServerFactory creates a new HTTP server instance
func ServerFactory(host string, port int) *Server {
	return &Server{
		Host:     host,
		Port:     port,
		Handlers: make(map[string]RequestHandler), //just alloc the space for now
	}
}

// RegisterHandler registers a handler for a specific HTTP method and path
func (s *Server) RegisterHandler(method, path string, handler RequestHandler) {
	key := method + " " + path
	s.Handlers[key] = handler
	log.Printf("Registered handler for %s %s", method, path)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.mutex.Lock()
	if s.running {
		s.mutex.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mutex.Unlock()

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	var err error
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		s.running = false
		return fmt.Errorf("error starting server on %s: %w", addr, err)
	}

	log.Printf("Server started on %s", addr)

	//accept connections in a goroutine
	go s.acceptConnections()

	return nil
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return fmt.Errorf("server not running")
	}

	err := s.listener.Close()
	s.running = false

	//wait for all connections to finish
	s.wg.Wait()
	log.Printf("Server stopped")

	return err
}

// acceptConnections accepts new connections and handles them
func (s *Server) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			//check if the server is shutting down
			s.mutex.Lock()
			running := s.running
			s.mutex.Unlock()

			//the server has shut down so we dont have any errors to print
			if !running {
				break
			}

			log.Printf("Error accepting connection: %v", err)
			continue
		}

		//handle each connection in a separate goroutine
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()

			s.handleConnection(c)
		}(conn)
	}
}

// handleConnection processes an individual HTTP connection
func (s *Server) handleConnection(conn net.Conn) {
	//set a read timeout
	err := conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err != nil {
		log.Printf("Error setting read deadline: %v", err)
		return
	}

	//parse the request
	req, err := ParseRequest(conn)
	if err != nil {
		log.Printf("Error parsing request: %v", err)
		resp := NewResponse(StatusBadRequest)
		resp.SetBodyString(fmt.Sprintf("Bad request: %v", err))
		resp.Write(conn)
		return
	}

	log.Printf("Received request: %s %s", req.Method, req.Path)

	//find and execute the handler
	handlerKey := fmt.Sprintf("%s %s", req.Method, req.Path)
	handler, ok := s.Handlers[handlerKey]

	//try a wildcard handler if specific handler not found
	if !ok {
		handlerKey = fmt.Sprintf("%s *", req.Method)
		handler, ok = s.Handlers[handlerKey]
	}

	var resp *Response
	if ok {
		resp = handler(req)
	} else {
		//no handler found
		resp = NewResponse(StatusNotFound)
		resp.SetBodyString(fmt.Sprintf("No handler for %s %s", req.Method, req.Path))
	}

	err = resp.Write(conn)
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
}
