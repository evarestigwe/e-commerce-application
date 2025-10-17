FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o product-service .

FROM gcr.io/distroless/base-debian11
COPY --from=builder /app/product-service /
EXPOSE 8002
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/product-service", "-health"]
ENTRYPOINT ["/product-service"]