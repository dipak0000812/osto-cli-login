FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o osto-auth .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S appgroup -g 10001 \
    && adduser -S appuser -u 10001 -G appgroup
WORKDIR /app/
COPY --from=builder /app/osto-auth .
USER appuser
CMD ["./osto-auth"]