# internal/quic

This directory now contains the first concrete QUIC foundation pieces for Triton:

- `packet/varint.go`: RFC 9000 variable-length integer encoding/decoding
- `packet/number.go`: packet number truncation and reconstruction helpers
- `transport/udp.go`: reusable UDP transport wrapper with pooled buffers

These are intentionally small, tested building blocks for the larger custom QUIC engine described in `.project/SPECIFICATION.md`.
