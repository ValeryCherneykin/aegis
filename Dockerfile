# Step 1: Fetch certificates and timezone data
FROM alpine:3.19 AS certs
RUN apk add --no-cache ca-certificates tzdata

# Step 2: Build the minimal scratch image
FROM scratch

# Copy certificates and timezone info
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary from the current directory
COPY aegis /aegis
COPY policies/ /policies/
COPY policies/refund_v1.yaml /refund_v1.yaml

USER 10001:10001

# Expose entrypoint
ENTRYPOINT ["/aegis"]
CMD ["-serve", "-policy", "/refund_v1.yaml"]
