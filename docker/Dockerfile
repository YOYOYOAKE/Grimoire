FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /opt/grimoire

COPY dist/grimoire-bot /opt/grimoire/grimoire-bot
COPY docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /opt/grimoire/grimoire-bot /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
