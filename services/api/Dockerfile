# syntax=docker/dockerfile:1
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/commerceops-api ./cmd/server

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl poppler-utils tesseract-ocr tesseract-ocr-eng \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --no-create-home --shell /usr/sbin/nologin commerceops
COPY --from=build /out/commerceops-api /usr/local/bin/commerceops-api
ENV HOME=/tmp XDG_CACHE_HOME=/tmp/cache
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/commerceops-api"]
