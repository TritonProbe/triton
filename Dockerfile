FROM golang:1.24 AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/triton ./cmd/triton

FROM debian:bookworm-slim
COPY --from=builder /out/triton /triton
EXPOSE 8443 9090
ENTRYPOINT ["/triton"]
