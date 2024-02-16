# Name the node stage "builder"
FROM node:18.12.1-alpine AS builder

# copy only the package.json and yarn.lock to work directory so that they can be cached
COPY package.json /tmp/package.json
COPY yarn.lock /tmp/yarn.lock
COPY .yarn /tmp/.yarn
COPY .yarnrc.yml /tmp/.yarnrc.yml
# install node modules
RUN cd /tmp && yarn install
RUN mkdir -p /app && cp -a /tmp/node_modules /app/

# Set working directory
WORKDIR /app
# Copy all files from current directory to working dir in image
COPY . .
# Build app for production with minification
RUN yarn build

# nginx state for serving content
FROM nginxinc/nginx-unprivileged:1.25.4-alpine
# Set working directory to nginx asset directory
WORKDIR /usr/share/nginx/html
# Copy static assets from builder stage
COPY --from=builder /app/build .
# Copy nginx.conf
COPY ./nginx.conf /etc/nginx/conf.d/default.conf
# Containers run nginx with global directives and daemon off
ENTRYPOINT ["sh", "-c", "cd /usr/share/nginx/html/ && nginx -g 'daemon off;'"]
