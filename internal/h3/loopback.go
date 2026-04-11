package h3

import (
	"bytes"
	"io"

	h3frame "github.com/tritonprobe/triton/internal/h3/frame"
	"github.com/tritonprobe/triton/internal/quic/connection"
	quicframe "github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/stream"
	"github.com/tritonprobe/triton/internal/quic/transport"
)

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

func RoundTripLoopback(listener *transport.Listener, session *transport.Session, serverConn *connection.Connection, method, path string, body []byte) (*Response, error) {
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

	serverStream, err := acceptStream(serverConn.Streams())
	if err != nil {
		return nil, err
	}
	reqBytes, err := readAll(serverStream)
	if err != nil {
		return nil, err
	}
	reqFrames, err := h3frame.Parse(reqBytes)
	if err != nil {
		return nil, err
	}

	requestHeaders := map[string]string{}
	var requestBody []byte
	for _, f := range reqFrames {
		switch ft := f.(type) {
		case h3frame.HeadersFrame:
			requestHeaders = DecodeHeaders(ft.Block)
		case h3frame.DataFrame:
			requestBody = append(requestBody, ft.Data...)
		}
	}

	responseHeaders := map[string]string{
		":status": "200",
		"x-path":  requestHeaders[":path"],
		"x-body":  string(requestBody),
	}
	responsePayload, err := h3frame.Encode([]h3frame.Frame{
		h3frame.HeadersFrame{Block: EncodeHeaders(responseHeaders)},
		h3frame.DataFrame{Data: []byte("ok:" + requestHeaders[":method"] + ":" + requestHeaders[":path"])},
	})
	if err != nil {
		return nil, err
	}

	if err := listener.SendFrames(serverConn, []quicframe.Frame{
		quicframe.StreamFrame{StreamID: 0, Offset: 0, Data: responsePayload, Fin: true},
	}); err != nil {
		return nil, err
	}
	if err := session.ReceiveFrames(); err != nil {
		return nil, err
	}

	return readClientResponse(session)
}

func acceptStream(m *stream.Manager) (*stream.Stream, error) {
	return m.AcceptStream()
}

func readAll(s *stream.Stream) ([]byte, error) {
	var out bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return nil, writeErr
			}
		}
		if err == io.EOF {
			return out.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return out.Bytes(), nil
		}
	}
}
