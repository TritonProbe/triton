FROM golang:1.24 AS builder
WORKDIR /src
ARG VERSION=dev
ARG BUILD_TIME=unknown
ENV CGO_ENABLED=0
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" -o /out/triton ./cmd/triton

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --uid 10001 --create-home --home-dir /var/lib/triton --shell /usr/sbin/nologin triton \
  && mkdir -p /var/lib/triton/triton-data /var/lib/triton/traces \
  && chown -R triton:triton /var/lib/triton
WORKDIR /var/lib/triton
COPY --from=builder /out/triton /usr/local/bin/triton
USER 10001:10001
EXPOSE 4434/udp 8443/tcp 9090/tcp
ENTRYPOINT ["triton"]
CMD ["server"]
