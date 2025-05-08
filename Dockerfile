FROM golang:1.24.2-alpine AS builder

WORKDIR /app

#copy go module files
COPY go.mod ./

#download dependencies
RUN go mod download

#copy source code
COPY . .

#build the applications
RUN go build -o /app/bin/server ./cmd/server
RUN go build -o /app/bin/gateway ./cmd/gateway
RUN go build -o /app/bin/sensor ./cmd/sensor

#create a minimal runtime image
FROM alpine:latest

WORKDIR /app

#copy binaries from builder stage
COPY --from=builder /app/bin/server /app/bin/server
COPY --from=builder /app/bin/gateway /app/bin/gateway
COPY --from=builder /app/bin/sensor /app/bin/sensor

#set executable permissions
RUN chmod +x /app/bin/server
RUN chmod +x /app/bin/gateway
RUN chmod +x /app/bin/sensor

#create a non-root user
RUN adduser -D -h /app appuser
USER appuser

#default command (will be overridden by docker-compose)
CMD ["/app/bin/server"]