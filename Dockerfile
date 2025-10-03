# Build stage
FROM golang:1.24 AS build
WORKDIR /app
COPY . .
RUN go build -o /zephyr ./cmd/server

# Runtime stage
FROM debian:stable-slim

# Install wget for healthcheck
RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/*

# Copy binary from builder
COPY --from=build /zephyr /usr/local/bin/zephyr

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8080

# Use entrypoint to set dynamic environment variables
ENTRYPOINT ["/entrypoint.sh"]

# Run the application
CMD ["/usr/local/bin/zephyr"]
