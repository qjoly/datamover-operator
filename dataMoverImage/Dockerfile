FROM alpine
RUN apk add --no-cache \
    rclone \
    curl \
    jq \
    bash \
    && rm -rf /var/cache/apk/*
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]