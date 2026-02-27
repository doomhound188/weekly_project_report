# ── Build Stage ──────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /weekly-report .

# ── Runtime Stage ────────────────────────────────────────────────
FROM alpine:3.21

# Timezone
ENV TZ=America/Toronto
RUN apk add --no-cache tzdata && \
    ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && \
    echo $TZ > /etc/timezone

WORKDIR /app

# Copy binary and web assets
COPY --from=builder /weekly-report /app/weekly-report
COPY web/ /app/web/

# Expose web port
EXPOSE 5000

ENTRYPOINT ["/app/weekly-report"]
