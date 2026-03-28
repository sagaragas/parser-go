package server

import (
	"net/http"
)

// Server holds the HTTP server dependencies.
// This is a placeholder that will be expanded with actual service endpoints.
type Server struct {
	mux *http.ServeMux
}

// New creates a new server.
func New() *Server {
	s := &Server{
		mux: http.NewServeMux(),
	}
	return s
}

// Handler returns the server's HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}
