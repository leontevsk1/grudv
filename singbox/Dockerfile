# Официальный образ sing-box собран без with_v2ray_api,
# а он нужен nup'у для per-user статистики трафика.
FROM golang:1.25-alpine AS build
RUN CGO_ENABLED=0 go install -tags "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_tailscale,with_v2ray_api" github.com/sagernet/sing-box/cmd/sing-box@v1.13.14

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /go/bin/sing-box /usr/local/bin/sing-box
ENTRYPOINT ["sing-box"]
