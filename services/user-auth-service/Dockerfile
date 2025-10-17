# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o user-auth-service .

# Final stage - distroless
FROM gcr.io/distroless/base-debian11

COPY --from=builder /app/user-auth-service /

EXPOSE 8001

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/user-auth-service", "-health"]

ENTRYPOINT ["/user-auth-service"]