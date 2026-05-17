FROM golang:1.22-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/milo-relay ./cmd/milo-relay

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/milo-relay /milo-relay
EXPOSE 8080
ENTRYPOINT ["/milo-relay"]
