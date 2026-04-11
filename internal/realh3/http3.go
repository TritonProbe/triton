package realh3

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/quic-go/quic-go"
	quichttp3 "github.com/quic-go/quic-go/http3"

	"github.com/tritonprobe/triton/internal/observability"
)

func NewClient(timeout time.Duration, insecure bool, traceDir string) (*http.Client, *quichttp3.Transport) {
	transport := &quichttp3.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: insecure,
		},
		QUICConfig: &quic.Config{
			Tracer: observability.NewQLOGTracer(traceDir),
		},
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	return client, transport
}
