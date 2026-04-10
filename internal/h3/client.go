package h3

import (
	"github.com/tritonprobe/triton/internal/quic/transport"
)

type Client struct {
	session *transport.Session
}

func NewClient(session *transport.Session) *Client {
	return &Client{session: session}
}

func (c *Client) Session() *transport.Session {
	return c.session
}
