# Development image for the Go server. It rebuilds and restarts the binary
# whenever a source file under the bind-mounted /app changes.
FROM golang:1.26-alpine

RUN apk add --no-cache git

RUN go install github.com/air-verse/air@v1.67.4

WORKDIR /app

ENV CGO_ENABLED=0

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
