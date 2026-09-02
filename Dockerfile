FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /app -ldflags="-s -w" ./cmd/gradeserver

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /usr/local/bin/gradeserver
ENV ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gradeserver"]
