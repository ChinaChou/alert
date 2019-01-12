FROM alpine
COPY bin/alert /
ENTRYPOINT /alert
