# Build stage
FROM golang:1.24-alpine AS build
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN go build -o webrtc-bridge .

# Runtime stage
FROM alpine:latest
WORKDIR /app

# Copy the binary and config
COPY --from=build /app/webrtc-bridge .
COPY config.yml .

# Run the application
CMD ["./webrtc-bridge"]
