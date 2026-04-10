package h3

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	h3frame "github.com/tritonprobe/triton/internal/h3/frame"
	"github.com/tritonprobe/triton/internal/quic/connection"
	quicframe "github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/stream"
	"github.com/tritonprobe/triton/internal/quic/transport"
)

func RoundTripHandlerLoopback(listener *transport.Listener, session *transport.Session, serverConn *connection.Connection, handler http.Handler, method, path string, body []byte) (*Response, error) {
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
	reqFrames, err := parseRequestFrames(serverStream)
	if err != nil {
		return nil, err
	}
	req, err := buildRequest(reqFrames)
	if err != nil {
		return nil, err
	}

	recorder := newResponseRecorder()
	handler.ServeHTTP(recorder, req)

	responsePayload, err := h3frame.Encode([]h3frame.Frame{
		h3frame.HeadersFrame{Block: EncodeHeaders(responseHeaders(recorder.header, recorder.statusCode))},
		h3frame.DataFrame{Data: recorder.body.Bytes()},
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

func parseRequestFrames(s *stream.Stream) ([]h3frame.Frame, error) {
	reqBytes, err := readAll(s)
	if err != nil {
		return nil, err
	}
	return h3frame.Parse(reqBytes)
}

func buildRequest(frames []h3frame.Frame) (*http.Request, error) {
	headers := map[string]string{}
	var body []byte
	for _, f := range frames {
		switch ft := f.(type) {
		case h3frame.HeadersFrame:
			headers = DecodeHeaders(ft.Block)
		case h3frame.DataFrame:
			body = append(body, ft.Data...)
		}
	}
	method := headers[":method"]
	if method == "" {
		method = http.MethodGet
	}
	path := headers[":path"]
	if path == "" {
		path = "/"
	}
	req, err := http.NewRequest(method, "https://loopback"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = ToHTTPHeader(headers)
	return req, nil
}

func responseHeaders(h http.Header, status int) map[string]string {
	out := map[string]string{
		":status": strconv.Itoa(status),
	}
	for key, values := range h {
		if len(values) == 0 {
			continue
		}
		out[http.CanonicalHeaderKey(key)] = values[0]
	}
	return out
}

func readClientResponse(session *transport.Session) (*Response, error) {
	clientStream, ok := session.Connection().Streams().GetStream(0)
	if !ok {
		return nil, io.EOF
	}
	respBytes, err := readAll(clientStream)
	if err != nil {
		return nil, err
	}
	respFrames, err := h3frame.Parse(respBytes)
	if err != nil {
		return nil, err
	}
	resp := &Response{StatusCode: 200, Headers: map[string]string{}}
	for _, f := range respFrames {
		switch ft := f.(type) {
		case h3frame.HeadersFrame:
			resp.Headers = DecodeHeaders(ft.Block)
			if code, err := strconv.Atoi(resp.Headers[":status"]); err == nil {
				resp.StatusCode = code
			}
		case h3frame.DataFrame:
			resp.Body = append(resp.Body, ft.Data...)
		}
	}
	return resp, nil
}

type responseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}
