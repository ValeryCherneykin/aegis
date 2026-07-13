# Stage: Final runtime image
FROM alpine:3.19

# Install necessary certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Copy the binary that GoReleaser already built for us
COPY aegis /aegis
# Copy policies from the extra_files included in the context
COPY policies/ /policies/

# Security best practice: run as non-root user
USER 10001:10001

ENTRYPOINT ["/aegis"]
CMD ["-serve", "-policy", "/policies/refund_v1.yaml"]
