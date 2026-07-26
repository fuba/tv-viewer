FROM nginx:1.30.4-alpine3.24

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf
RUN chown -R nginx:nginx /var/cache/nginx \
    && touch /var/run/nginx.pid \
    && chown nginx:nginx /var/run/nginx.pid

USER nginx

EXPOSE 18090

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider --header='Host: puma2:18090' http://127.0.0.1:18090/

CMD ["nginx", "-g", "daemon off;"]
