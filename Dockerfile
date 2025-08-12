
FROM golang:1.22 AS build
WORKDIR /app
COPY . .
RUN go build -o /zephyr ./cmd/server

FROM debian:stable-slim
COPY --from=build /zephyr /usr/local/bin/zephyr
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/zephyr"]
