# Development image for the Next.js app. Dependencies are installed into the
# image so that a fresh clone does not need a local `npm install`.
FROM node:22-alpine

WORKDIR /app

# The app sources arrive as a bind mount at runtime, so the postinstall
# `next typegen` has nothing to read at build time; `next dev` regenerates
# the types when the container starts.
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts

EXPOSE 3000

CMD ["npm", "run", "dev", "--", "--hostname", "0.0.0.0"]
