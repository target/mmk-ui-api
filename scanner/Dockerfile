FROM node:14.18.1-alpine
MAINTAINER Merrymaker Team "merrymaker@target.com"

RUN apk add --no-cache -X http://dl-cdn.alpinelinux.org/alpine/edge/testing \
      yara-dev

RUN apk add wget ca-certificates
RUN update-ca-certificates --fresh

RUN apk add --no-cache --virtual .gyp \
  bash \
  git \
  openssh \
  python3 \
  make \
  g++

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
