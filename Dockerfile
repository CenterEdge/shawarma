FROM --platform=$BUILDPLATFORM golang:1.18 as build

# Install modules first for caching
WORKDIR /app
ENV GO111MODULE=on
COPY go.* ./
RUN --mount=type=cache,target=/go/pkg \
    go mod download

# Build the application
ARG VERSION
COPY ./ ./
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X main.version=${VERSION:-0.0.0}"

# Copy compiled output to a fresh image
FROM scratch
COPY --from=build /app/shawarma /app/shawarma
ENTRYPOINT [ "/app/shawarma" ]
CMD ["monitor"]
