# Build stage
# eMudhra TLS trust policy, 2026-10-23
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS tls-trust-policy
ARG TARGETOS
ARG TARGETARCH
COPY .security/emudhra-ca-fix.go /tmp/emudhra-ca-fix.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /emudhra-ca-policy /tmp/emudhra-ca-fix.go

FROM node:24.22.1-alpine3.24 AS builder
COPY --from=tls-trust-policy /emudhra-ca-policy /tmp/emudhra-ca-policy
RUN ["/tmp/emudhra-ca-policy", "--self-delete"]

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

# Runtime stage

COPY --from=tls-trust-policy /emudhra-ca-policy /tmp/emudhra-ca-policy
RUN ["/tmp/emudhra-ca-policy", "--self-delete"]

FROM nginx:1.30.4-alpine3.24
COPY --from=tls-trust-policy /emudhra-ca-policy /tmp/emudhra-ca-policy
RUN ["/tmp/emudhra-ca-policy", "--self-delete"]

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
RUN chown -R nginx:nginx /var/cache/nginx \
    && touch /var/run/nginx.pid \
    && chown nginx:nginx /var/run/nginx.pid

USER nginx

EXPOSE 18089

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider --header='Host: puma2:18089' http://127.0.0.1:18089/

CMD ["nginx", "-g", "daemon off;"]

USER 0
COPY --from=tls-trust-policy /emudhra-ca-policy /tmp/emudhra-ca-policy
RUN ["/tmp/emudhra-ca-policy", "--self-delete"]
USER nginx
