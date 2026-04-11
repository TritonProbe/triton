package server

import (
	"net/http"

	"github.com/tritonprobe/triton/internal/appmux"
)

func NewMux() http.Handler {
	return appmux.New()
}
