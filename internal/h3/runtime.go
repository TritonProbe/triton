package h3

import (
	"errors"
	"net/http"
	"strings"
	"time"

	h3frame "github.com/tritonprobe/triton/internal/h3/frame"
	"github.com/tritonprobe/triton/internal/quic/connection"
	quicframe "github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/transport"
)

type UDPServer struct {
	listener *transport.Listener
	handler  http.Handler
}

func NewUDPServer(listener *transport.Listener, handler http.Handler) *UDPServer {
	return &UDPServer{
		listener: listener,
		handler:  handler,
	}
}

func (s *UDPServer) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "listener closed") {
				return nil
			}
			return err
		}
		go s.handleConnection(conn)
	}
}

func (s *UDPServer) handleConnection(conn *connection.Connection) {
	serverStream, err := acceptStream(conn.Streams())
	if err != nil {
		return
	}
	reqFrames, err := parseRequestFrames(serverStream)
	if err != nil {
		return
	}
	req, err := buildRequest(reqFrames)
	if err != nil {
		return
	}
	if remoteAddr := s.listener.RemoteAddr(conn); remoteAddr != nil {
		req.RemoteAddr = remoteAddr.String()
	}

	recorder := newResponseRecorder()
	s.handler.ServeHTTP(recorder, req)

	responsePayload, err := h3frame.Encode([]h3frame.Frame{
		h3frame.HeadersFrame{Block: EncodeHeaders(responseHeaders(recorder.header, recorder.statusCode))},
		h3frame.DataFrame{Data: recorder.body.Bytes()},
	})
	if err != nil {
		return
	}
	_ = s.listener.SendFrames(conn, []quicframe.Frame{
		quicframe.StreamFrame{StreamID: 0, Offset: 0, Data: responsePayload, Fin: true},
	})
}

func RoundTripAddress(address, method, path string, body []byte, timeout time.Duration) (*Response, error) {
	dialer := transport.NewDialer(timeout)
	session, err := dialer.DialSession(address)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	requestPayload, err := h3frame.Encode([]h3frame.Frame{
		h3frame.HeadersFrame{Block: EncodeHeaders(map[string]string{
			":method": method,
			":path":   path,
		})},
		h3frame.DataFrame{Data: body},
	})
	if err != nil {
		return nil, err
	}
	if err := session.SendStream(0, requestPayload, true); err != nil {
		return nil, err
	}
	if err := session.ReceiveFrames(); err != nil {
		return nil, err
	}
	resp, err := readClientResponse(session)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 100 {
		return nil, errors.New("invalid h3 response")
	}
	return resp, nil
}
