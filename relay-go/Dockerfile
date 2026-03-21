FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git (needed for go mod download with some modules)
RUN apk add --no-cache git

COPY go.mod ./
# go.sum is optional here — go mod tidy will create it if missing
COPY go.sum* ./

# Download dependencies (creates go.sum if missing)
RUN go mod download || go mod tidy

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /milo-relay \
    ./cmd/milo-relay

# ─── Final image ──────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /milo-relay /app/milo-relay

EXPOSE 8080

ENTRYPOINT ["/app/milo-relay"]
