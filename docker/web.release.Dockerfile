# Release image for the Next.js app, run on ECS Fargate (linux/arm64).
# NEXT_PUBLIC_* values are inlined into the browser bundle and the statically
# generated pages at build time, so each environment gets its own image.
# Build it from the repository root:
#   docker buildx build --platform linux/arm64 --provenance=false \
#     --build-arg NEXT_PUBLIC_SITE_URL=https://example.com \
#     --build-arg NEXT_PUBLIC_API_BASE_URL=https://api.example.com \
#     -f docker/web.release.Dockerfile -t <repository>:<tag> .
# The build stage runs on the build platform, so native optional dependencies
# such as sharp are installed for that platform. The app does not use the
# next/image optimizer, which is the only thing that loads sharp.
FROM --platform=$BUILDPLATFORM node:22-alpine AS build

WORKDIR /app

ENV NEXT_TELEMETRY_DISABLED=1

COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts

COPY web ./

ARG NEXT_PUBLIC_SITE_URL
ARG NEXT_PUBLIC_API_BASE_URL

# 未指定のまま通すと localhost を指すバンドルができる。ビルドの時点で止める。
RUN test -n "$NEXT_PUBLIC_SITE_URL" && test -n "$NEXT_PUBLIC_API_BASE_URL" \
    || { echo "NEXT_PUBLIC_SITE_URL と NEXT_PUBLIC_API_BASE_URL を --build-arg で渡す" >&2; exit 1; }

RUN npm run build

FROM node:22-alpine

WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    HOSTNAME=0.0.0.0 \
    PORT=3000

# standalone の出力は静的ファイルを含まないため、.next/static を隣に置く。
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static

USER node

EXPOSE 3000

CMD ["node", "server.js"]
