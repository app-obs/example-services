# Multi-stage build for product-service
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install git (needed for go mod download)
RUN apk add --no-cache git

# Try to cache modules. This is only possible when go.mod and go.sum is correct.
# If not, you have to enable "rebuild go.mod" below
COPY go.mod go.sum .
RUN go mod download

# Copy source code
COPY . .

# Declare build arguments
ARG APM_TYPE=none
ARG METRICS_TYPE=none

# Build the application
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    BUILD_TAGS=$APM_TYPE && \
    if [ "$METRICS_TYPE" = "otlp" ]; then BUILD_TAGS="$BUILD_TAGS,metrics"; fi && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -trimpath -tags="$BUILD_TAGS" -o main .

# Final stage - use minimal base image
FROM alpine:latest

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Expose port
EXPOSE 8086

# Run the binary
CMD ["./main"]