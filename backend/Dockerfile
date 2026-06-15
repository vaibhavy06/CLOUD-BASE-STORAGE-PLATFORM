# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Run stage
FROM alpine:latest

# Install Tesseract OCR and certificate authorities
RUN apk add --no-cache tesseract-ocr ca-certificates

WORKDIR /root/

# Copy the built binary
COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]
