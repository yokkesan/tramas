FROM caddy:2-alpine@sha256:eb4d35b7452d1ab6171fbdbe8d9275b2301dad6dad92115d91e19fb9b24d510f
RUN setcap -r /usr/bin/caddy && addgroup -g 1000 app && adduser -D -H -u 1000 -G app app && chown -R 1000:1000 /data /config
USER 1000:1000
