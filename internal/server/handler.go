package server

import "net/http"

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /models", s.withAuth(s.handleModels))
	mux.HandleFunc("GET /v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("POST /responses", s.withAuth(s.handleResponses))
	mux.HandleFunc("POST /v1/responses", s.withAuth(s.handleResponses))
	mux.HandleFunc("POST /v1/chat/completions", s.withAuth(s.handleChatCompletions))
	mux.HandleFunc("POST /v1/messages", s.withAuth(s.handleAnthropicMessages))
	return mux
}
