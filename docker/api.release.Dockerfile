# Release image for the Go server, run on ECS Fargate (linux/arm64).
# Build it from the repository root:
#   docker buildx build --platform linux/arm64 --provenance=false \
#     -f docker/api.release.Dockerfile -t <repository>:<tag> .
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

COPY db ./db

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/server ./cmd/migrate

# The static distroless image carries CA certificates and tzdata, and runs as
# an unprivileged user. It has no shell.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/server /out/migrate /

EXPOSE 8080

# ECS の RunTask で上書きできるのは CMD だけのため、ENTRYPOINT は使わない。
# デプロイはマイグレーションを command ["/migrate", "up"] で流す。
CMD ["/server"]
