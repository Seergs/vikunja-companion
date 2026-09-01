# Single static binary, distroless. CGO-free so the container needs no libc.
FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN go build -ldflags "-X main.Version=${VERSION}" -o /out/companion ./cmd/companion

# Staged so the default DB path (/data/companion.db) works even when no volume
# is mounted. 65532 is distroless's nonroot uid/gid.
RUN mkdir -p /data && chown 65532:65532 /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/companion /companion
COPY --from=build --chown=65532:65532 /data /data
EXPOSE 8080
ENTRYPOINT ["/companion"]
