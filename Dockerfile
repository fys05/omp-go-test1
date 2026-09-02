FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /app -ldflags="-s -w" ./cmd/gradeserver

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /usr/local/bin/gradeserver
ENV LISTEN_ADDR=:8080 DATA_FILE=/data/grades.json
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gradeserver"]
