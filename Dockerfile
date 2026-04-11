FROM golang:1.24 AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/triton ./cmd/triton

FROM debian:bookworm-slim
RUN useradd --system --uid 10001 triton
COPY --from=builder /out/triton /triton
USER triton
EXPOSE 4433/udp 8443/tcp 9090/tcp
ENTRYPOINT ["/triton"]
