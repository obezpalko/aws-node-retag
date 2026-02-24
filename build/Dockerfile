# Stage 1: Build static binary
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Download dependencies first (layer-cached separately from source)
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -extldflags=-static" \
    -trimpath \
    -o /aws-node-retag \
    ./cmd/aws-node-retag/

# Stage 2: Distroless runtime — no shell, no package manager, nonroot by default
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /aws-node-retag /aws-node-retag

USER nonroot:nonroot

ENTRYPOINT ["/aws-node-retag"]
