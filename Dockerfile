# Dockerfile — for LOCAL development: `docker build .` / `docker compose build`
# compiles the binary from source, right here.
#
# This is a DIFFERENT file from Dockerfile.goreleaser on purpose. GoReleaser
# already builds your binaries in the "builds:" step of .goreleaser.yaml —
# its Docker stage only ever COPYs a prebuilt binary in, it never runs
# `go build` again. Reusing one Dockerfile for both jobs is exactly what
# caused both errors you hit:
#   - locally: COPY aegis /aegis failed because nothing had built it yet
#   - in CI:   go build failed because GoReleaser's docker context is a
#              temp dir with no go.mod in it, not your repo root
# Two files, two jobs, both boring and correct.

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/aegis ./cmd/aegis

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/aegis /aegis
COPY policies/ /policies/
USER 10001:10001
ENTRYPOINT ["/aegis"]
CMD ["-serve", "-policy", "/policies/refund_v1.yaml"]
