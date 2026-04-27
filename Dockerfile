FROM node:22-alpine AS frontend-builder

WORKDIR /src/public

COPY public/package*.json ./
RUN npm ci

COPY public/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-builder

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /src/public/out ./public/out

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/qwen2api ./cmd/qwen2api

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /out/qwen2api /app/qwen2api
COPY --from=go-builder /src/public/out /app/public/out
COPY README.md /app/README.md

EXPOSE 3000

ENTRYPOINT ["/app/qwen2api"]
