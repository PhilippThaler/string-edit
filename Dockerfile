# Stage 1: Build the application
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the binary
# CGO_ENABLED=0 disables C dependencies, allowing a pure Go build
# This works because we switched to 'modernc.org/sqlite'
RUN CGO_ENABLED=0 GOOS=linux go build -o site .

# Stage 2: Create the minimal runtime image
FROM alpine:3.21.3

WORKDIR /app

# Install tzdata for timezone support
RUN apk add --no-cache tzdata=2026a-r0

# Create a non-root user and set permissions for the app and data directories
RUN addgroup -S appgroup && adduser -S appuser -G appgroup && \
    mkdir -p /app/data && \
    chown -R appuser:appgroup /app

# Copy the binary from the builder stage
COPY --from=builder /app/site .
RUN chown appuser:appgroup /app/site

# Switch to the non-root user
USER appuser

# Expose the application port
EXPOSE 8080

# Create a volume for the database so data persists
VOLUME /app/data

# Set default environment variables for database connection
ENV DB_TYPE="sqlite"
ENV DB_NAME="data/history.db"

# Run the application
CMD ["./site"]
