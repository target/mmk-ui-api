FROM node:14.18.1-alpine
MAINTAINER Merrymaker Team "merrymaker@target.com"

RUN apk add --no-cache bash \
  git \
  openssl \
  wget \
  ca-certificates \
  openssh

RUN apk add --no-cache --virtual .gyp \
  python3 \
  make \
  g++

RUN apk add wget ca-certificates
RUN update-ca-certificates --fresh

ENV APP_HOME=/app

WORKDIR $APP_HOME
COPY package.json yarn.lock $APP_HOME/

COPY ./frontend/ $APP_HOME/frontend/

ARG PLUGIN_TAG

ENV VUE_APP_VERSION=$PLUGIN_TAG

RUN yarn workspace frontend install

WORKDIR $APP_HOME/frontend

RUN yarn build

FROM abiosoft/caddy:0.11.5-no-stats

COPY --from=0 /app/frontend/dist /srv/

COPY ./frontend/Caddyfile /etc/Caddyfile

EXPOSE 2015
