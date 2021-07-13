FROM golang:1.16 as build

# Install upx to compress executable after compilation
RUN apt-get update && \
    apt-get install -y xz-utils && \
    mkdir -p /opt/upx && \
    cd /opt/upx && \
    wget -q https://github.com/upx/upx/releases/download/v3.96/upx-3.96-amd64_linux.tar.xz -O ./upx.tar.xz && \
    tar --strip-components 1 -xf upx.tar.xz && \
    rm -rf /var/lib/apt/lists/* upx.tar.xz

# Install modules first for caching
WORKDIR /app
ENV GO111MODULE=on
COPY go.* ./
RUN go mod download

# Build the application, and compress with upx
ARG VERSION
COPY ./ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=${VERSION:-0.0.0}" && \
    /opt/upx/upx shawarma

# Copy compiled output to a fresh image
FROM scratch
COPY --from=build /app/shawarma /app/shawarma
ENTRYPOINT [ "/app/shawarma" ]
CMD ["monitor"]
