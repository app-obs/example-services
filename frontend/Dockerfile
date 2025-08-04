# Multi-stage build for frontend-service
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

# Uncomment these lines to rebuild go.mod, go.sum, and download missing dependencies
# helps when go.mod and go.sum is not updated yet in the repo
#RUN rm -f go.mod go.sum && \
#    go mod init frontend-service && \
#    go mod tidy

# Build the application
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -trimpath -o main .

# Final stage - use minimal base image
FROM alpine:latest

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Expose port
EXPOSE 8085

# Run the binary
CMD ["./main"]