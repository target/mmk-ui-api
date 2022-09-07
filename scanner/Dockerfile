FROM node:14.18.1-alpine
MAINTAINER Merrymaker Team "merrymaker@target.com"

RUN echo "http://dl-cdn.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories && \
    echo "http://dl-cdn.alpinelinux.org/alpine/edge/main" >> /etc/apk/repositories

RUN apk add --no-cache \
    libcrypto3 yara-dev make python3 g++ git curl

RUN update-ca-certificates --fresh

RUN mkdir -p /app

ENV APP_HOME=/app

WORKDIR $APP_HOME

COPY package.json yarn.lock $APP_HOME/

COPY ./scanner/package.json $APP_HOME/scanner/
COPY ./scanner/config $APP_HOME/scanner/config

RUN yarn workspace scanner install

COPY ./scanner/ $APP_HOME/scanner/

WORKDIR $APP_HOME/scanner

RUN yarn build

RUN rm -rf node_modules
RUN yarn workspace scanner install --prod

FROM node:14.18.1-alpine

RUN apk add --no-cache -X http://dl-cdn.alpinelinux.org/alpine/edge/testing \
      yara

RUN mkdir -p /app

COPY --from=0 /app/node_modules /app/node_modules
COPY --from=0 /app/scanner/dist /app
COPY --from=0 /app/scanner/package.json /app
COPY --from=0 /app/scanner/config /app/config

RUN addgroup -S -g 992 merrymaker
RUN adduser -S -G merrymaker merrymaker

RUN chown -R merrymaker:merrymaker /app

USER merrymaker

WORKDIR /app
CMD ["node", "/app/worker.js"]
