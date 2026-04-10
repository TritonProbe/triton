package h3

import (
	"net/http"

	"github.com/tritonprobe/triton/internal/quic/connection"
	"github.com/tritonprobe/triton/internal/quic/transport"
)

type Server struct {
	listener *transport.Listener
	conn     *connection.Connection
	handler  http.Handler
}

func NewServer(listener *transport.Listener, conn *connection.Connection, handler http.Handler) *Server {
	return &Server{
		listener: listener,
		conn:     conn,
		handler:  handler,
	}
}

func (s *Server) ServeRoundTrip(client *Client, method, path string, body []byte) (*Response, error) {
	return RoundTripHandlerLoopback(s.listener, client.Session(), s.conn, s.handler, method, path, body)
}
