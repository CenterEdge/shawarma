FROM golang:1.18 as build

# Install modules first for caching
WORKDIR /app
ENV GO111MODULE=on
COPY go.* ./
RUN go mod download

# Build the application
ARG VERSION
COPY ./ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION:-0.0.0}"

# Copy compiled output to a fresh image
FROM scratch
COPY --from=build /app/shawarma /app/shawarma
ENTRYPOINT [ "/app/shawarma" ]
CMD ["monitor"]
