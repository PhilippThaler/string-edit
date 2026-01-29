# Stage 1: Build the application
FROM golang:1.25-alpine AS builder

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
FROM alpine:latest

WORKDIR /app

# Install tzdata for timezone support
RUN apk add --no-cache tzdata

# Copy the binary and the template from the builder stage
COPY --from=builder /app/site .
COPY --from=builder /app/index.html .

# Expose the application port
EXPOSE 8080

# Create a volume for the database so data persists
VOLUME /data

# Run the application
CMD ["./site"]
