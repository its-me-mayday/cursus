FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o cursus ./cmd/cursus

FROM alpine:3.19

RUN addgroup -S cursus && adduser -S cursus -G cursus
USER cursus

WORKDIR /app
COPY --from=builder /app/cursus .

EXPOSE 8080

ENTRYPOINT ["./cursus"]
