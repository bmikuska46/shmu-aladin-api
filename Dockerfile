# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
	&& adduser -D -H -u 10001 appuser

WORKDIR /app
COPY --from=build /out/server /app/server

RUN mkdir -p /data && chown -R appuser:appuser /data /app

USER appuser

ENV ADDR=:8080 \
	DATABASE_PATH=/data/shmu.db \
	SYNC_FORECASTS=false

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/app/server"]
