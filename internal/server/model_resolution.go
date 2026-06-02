package server

import (
	"ilonasin/internal/routing"
)

func (s *Server) resolveModelAddress(model string) (routing.ModelAddress, error) {
	return routing.ParseModelAddress(model)
}
