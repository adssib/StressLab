# syntax=docker/dockerfile:1
# One build, one image per binary: `--target api` or `--target seed`.
# Static Go binaries on distroless: no shell, no package manager, non-root.

FROM golang:1.27.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -o /out/ ./cmd/api ./cmd/seed

FROM gcr.io/distroless/static-debian13:nonroot AS api
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
CMD ["-config", "/etc/stresslab/config.yaml"]

FROM gcr.io/distroless/static-debian13:nonroot AS seed
COPY --from=build /out/seed /usr/local/bin/seed
ENTRYPOINT ["/usr/local/bin/seed"]
CMD ["build"]
