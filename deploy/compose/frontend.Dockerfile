FROM node:20-alpine AS build
WORKDIR /src
COPY web/app/package.json ./package.json
COPY web/app/package-lock.json ./package-lock.json
COPY web/app/tsconfig.json ./tsconfig.json
COPY web/app/vite.config.ts ./vite.config.ts
COPY web/app/index.html ./index.html
COPY web/app/public ./public
COPY web/app/src ./src
RUN npm ci --no-audit --no-fund && npm run build

FROM nginx:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY deploy/compose/frontend.nginx.conf /etc/nginx/conf.d/tls.conf
COPY deploy/compose/frontend.nginx.http.conf /etc/nginx/conf.d/http.conf
COPY deploy/compose/frontend-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
# Default to HTTP; entrypoint switches to TLS if certs are mounted.
RUN cp /etc/nginx/conf.d/http.conf /etc/nginx/conf.d/default.conf
EXPOSE 80 443
ENTRYPOINT ["/entrypoint.sh"]
