# Build stage. Migrations are embedded into the binary via go:embed, so the
# runtime image is fully self-contained.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/nikolaymatrosov/nvelope/internal/service.Version=${VERSION}" \
    -o /out/migrate ./cmd/migrate

# Runtime stage: minimal, non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/migrate /migrate
USER nonroot:nonroot
ENTRYPOINT ["/migrate"]
