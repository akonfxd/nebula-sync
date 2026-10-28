# The console is built first and copied in, so the go build can embed it.
FROM node:22-alpine AS node

WORKDIR /app

COPY web/package.json web/package-lock.json ./web/
RUN npm --prefix web ci

COPY web ./web
RUN npm --prefix web run build

FROM golang:1.26-alpine AS golang

WORKDIR /app

RUN apk add -U tzdata && \
    apk --update add ca-certificates

COPY . .
COPY --from=node /app/internal/api/ui/dist ./internal/api/ui/dist

RUN go mod download
RUN go mod verify

ARG VERSION=dev

ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOFLAGS="-a -trimpath -ldflags=-w -ldflags=-s -ldflags=-X=github.com/lovelaze/nebula-sync/version.Version=${VERSION} -o=nebula-sync"

RUN go build .

# A writable home for the configuration database, owned by the runtime user.
RUN mkdir -p /data && chown 1001:1001 /data

FROM scratch

COPY --link --from=golang /usr/share/zoneinfo/ /usr/share/zoneinfo/
COPY --link --from=golang /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --link --from=golang /app/nebula-sync /usr/local/bin/
COPY --link --from=golang --chown=1001:1001 /data /data

USER 1001

ENV API_ENABLED=true
ENV DB_PATH=/data/nebula-sync.db

VOLUME /data

HEALTHCHECK --interval=30s --timeout=3s --start-period=3s --retries=3 \
  CMD ["nebula-sync", "healthcheck"]

ENTRYPOINT ["nebula-sync"]
CMD ["run"]
