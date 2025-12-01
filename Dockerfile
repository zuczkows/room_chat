# Template is from: https://docs.docker.com/guides/go-prometheus-monitoring/containerize/
# Use the official Golang image as the base
FROM golang:1.24-alpine AS builder

# Set environment variables
ENV CGO_ENABLED=0

# Set working directory inside the container
WORKDIR /build

# Copy go.mod and go.sum files for dependency installation
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire application source
COPY . .

# Build the Go binary
RUN go build -o app .

# Final lightweight stage
FROM alpine:3.22 AS final

WORKDIR /

# Copy the compiled binary from the builder stage
COPY --from=builder /build/app /

# Expose the application's port
EXPOSE 8000

# Run the application (correct path)
CMD ["/app", "-cfg=/config.json"]