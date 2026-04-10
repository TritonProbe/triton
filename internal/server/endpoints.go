package server

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func NewMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/echo", handleEcho)
	mux.HandleFunc("/download/", handleDownload)
	mux.HandleFunc("/upload", handleUpload)
	mux.HandleFunc("/delay/", handleDelay)
	mux.HandleFunc("/redirect/", handleRedirect)
	mux.HandleFunc("/streams/", handleStreams)
	mux.HandleFunc("/headers/", handleHeaders)
	mux.HandleFunc("/status/", handleStatus)
	mux.HandleFunc("/drip/", handleDrip)
	mux.HandleFunc("/tls-info", handleTLSInfo)
	mux.HandleFunc("/quic-info", handleQUICInfo)
	mux.HandleFunc("/migration-test", handleMigration)
	mux.HandleFunc("/.well-known/triton", handleCapabilities)
	return mux
}

func handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         "triton",
		"mode":         "server",
		"capabilities": []string{"http1", "http2", "dashboard", "probe-storage", "bench-storage"},
	})
}

func handlePing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "pong")
}

func handleEcho(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	writeJSON(w, http.StatusOK, map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"query":   r.URL.RawQuery,
		"headers": r.Header,
		"body":    string(body),
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	size, err := parseTailInt(r.URL.Path, "/download/")
	if err != nil || size <= 0 {
		http.Error(w, "invalid size", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	chunk := []byte("TRITONPROBE")
	remaining := size
	for remaining > 0 {
		writeLen := len(chunk)
		if writeLen > remaining {
			writeLen = remaining
		}
		if _, err := w.Write(chunk[:writeLen]); err != nil {
			return
		}
		remaining -= writeLen
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	n, _ := io.Copy(io.Discard, r.Body)
	writeJSON(w, http.StatusOK, map[string]any{
		"bytes":       n,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

func handleDelay(w http.ResponseWriter, r *http.Request) {
	ms, err := parseTailInt(r.URL.Path, "/delay/")
	if err != nil || ms < 0 {
		http.Error(w, "invalid delay", http.StatusBadRequest)
		return
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]any{"delay_ms": ms})
}

func handleHeaders(w http.ResponseWriter, r *http.Request) {
	n, err := parseTailInt(r.URL.Path, "/headers/")
	if err != nil || n < 0 {
		http.Error(w, "invalid header count", http.StatusBadRequest)
		return
	}
	for i := 0; i < n; i++ {
		w.Header().Set(fmt.Sprintf("X-Triton-%d", i), fmt.Sprintf("value-%d", i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"headers": n})
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	n, err := parseTailInt(r.URL.Path, "/redirect/")
	if err != nil || n < 0 {
		http.Error(w, "invalid redirect count", http.StatusBadRequest)
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"redirects": 0, "final": true})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/redirect/%d", n-1), http.StatusFound)
}

func handleStreams(w http.ResponseWriter, r *http.Request) {
	n, err := parseTailInt(r.URL.Path, "/streams/")
	if err != nil || n < 0 {
		http.Error(w, "invalid stream count", http.StatusBadRequest)
		return
	}
	streams := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		streams = append(streams, map[string]any{
			"id":           i + 1,
			"scheduled_ms": i * 25,
			"state":        "simulated",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requested": n,
		"streams":   streams,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	code, err := parseTailInt(r.URL.Path, "/status/")
	if err != nil || code < 100 || code > 599 {
		http.Error(w, "invalid status code", http.StatusBadRequest)
		return
	}
	writeJSON(w, code, map[string]any{"status": code})
}

func handleDrip(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/drip/"), "/")
	if len(parts) != 2 {
		http.Error(w, "expected /drip/:size/:delay", http.StatusBadRequest)
		return
	}
	size, err1 := strconv.Atoi(parts[0])
	delay, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || size < 0 || delay < 0 {
		http.Error(w, "invalid drip parameters", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	flusher, _ := w.(http.Flusher)
	for i := 0; i < size; i++ {
		_, _ = w.Write([]byte{byte('a' + rand.Intn(26))})
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

func handleTLSInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{"tls": false}
	if r.TLS != nil {
		info = map[string]any{
			"tls":        true,
			"version":    tlsVersion(r.TLS.Version),
			"cipher":     tls.CipherSuiteName(r.TLS.CipherSuite),
			"alpn":       r.TLS.NegotiatedProtocol,
			"server":     r.TLS.ServerName,
			"peer_certs": len(r.TLS.PeerCertificates),
		}
	}
	writeJSON(w, http.StatusOK, info)
}

func handleQUICInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": false,
		"message":   "custom QUIC engine not implemented yet in this scaffold",
	})
}

func handleMigration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": false,
		"message":   "connection migration requires QUIC transport",
	})
}

func handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      "triton",
		"version":   "dev",
		"protocols": []string{"http/1.1", "h2"},
		"endpoints": []string{"/ping", "/echo", "/download/:size", "/upload", "/delay/:ms", "/streams/:n", "/headers/:n", "/redirect/:n", "/status/:code", "/drip/:size/:delay"},
	})
}

func parseTailInt(path, prefix string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(path, prefix))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func tlsVersion(v uint16) string {
	switch v {
	case 0x0304:
		return "TLS1.3"
	case 0x0303:
		return "TLS1.2"
	default:
		return fmt.Sprintf("0x%x", v)
	}
}
