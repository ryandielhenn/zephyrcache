# Build stage
FROM golang:1.25-alpine AS build

# Install git and build dependencies
RUN apk add --no-cache git

WORKDIR /app
COPY . .

# Build the Go binary statically (ensures it runs on Alpine)
RUN CGO_ENABLED=0 GOOS=linux go build -o /zephyr ./cmd/server

# Runtime stage
FROM alpine:latest

# Copy binary from build stage
COPY --from=build /zephyr /usr/local/bin/zephyr

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8080

# Use entrypoint to set dynamic environment variables
ENTRYPOINT ["/entrypoint.sh"]

# Default command
CMD ["/usr/local/bin/zephyr"]
