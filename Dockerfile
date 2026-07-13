# --- STAGE 1: Build the binary ---
# Use an official Go image for a clean, reproducible build environment.
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container.
WORKDIR /app

# Copy the entire project to the container.
COPY . .

# Compile the binary with CGO disabled for a statically linked executable.
# This ensures maximum portability across different Linux distributions.
RUN CGO_ENABLED=0 GOOS=linux go build -o aegis ./cmd/aegis

# --- STAGE 2: Final runtime image ---
# Use a lightweight Alpine image to minimize the final attack surface and image size.
FROM alpine:3.19

# Install necessary certificates for HTTPS and timezone data for log/time accuracy.
RUN apk add --no-cache ca-certificates tzdata

# Copy only the compiled binary and policy files from the builder stage.
COPY --from=builder /app/aegis /aegis
COPY --from=builder /app/policies /policies

# Run the application as a non-privileged user for enhanced security.
USER 10001:10001

# Define the entrypoint and default command for the service.
ENTRYPOINT ["/aegis"]
CMD ["-serve", "-policy", "/policies/refund_v1.yaml"]
