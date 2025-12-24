FROM nginx:alpine

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

EXPOSE 18090

CMD ["nginx", "-g", "daemon off;"]