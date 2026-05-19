# Build stage.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
    -ldflags "-X github.com/nikolaymatrosov/nvelope/internal/service.Version=${VERSION}" \
    -o /out/scheduler ./cmd/scheduler

# Runtime stage: minimal, non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/scheduler /scheduler
USER nonroot:nonroot
ENTRYPOINT ["/scheduler"]
