# Multi-arch image carrying both binaries. The companion is the default
# entrypoint; the relay runs via `--entrypoint /relay` (docs/COMPANION.md §7).
FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN go build -ldflags "-X main.Version=${VERSION}" -o /out/companion ./cmd/companion && \
    go build -ldflags "-X main.Version=${VERSION}" -o /out/relay ./cmd/relay

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/companion /companion
COPY --from=build /out/relay /relay
EXPOSE 8080
ENTRYPOINT ["/companion"]
