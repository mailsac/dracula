FROM alpine as build

FROM scratch
ARG DRACULA_VERSION

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY ./out/dracula-server_linux-amd64-$DRACULA_VERSION /app/dracula-server
COPY ./out/dracula-cli_linux-amd64-$DRACULA_VERSION /app/dracula-cli
ENTRYPOINT ["/app/dracula-server"]
