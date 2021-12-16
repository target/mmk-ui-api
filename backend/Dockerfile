FROM node:14.18.1-alpine
MAINTAINER Merrymaker Team "merrymaker@target.com"

RUN apk add --no-cache --repository http://dl-cdn.alpinelinux.org/alpine/edge/main openssl
RUN apk add --no-cache -X http://dl-cdn.alpinelinux.org/alpine/edge/testing \
      yara

RUN apk add wget ca-certificates
RUN update-ca-certificates --fresh

RUN apk add --no-cache --virtual .gyp \
  bash \
  git \
  openssh \
  python3 \
  make \
  g++

ENV HOME=/home/merrymaker

RUN mkdir -p /home/merrymaker/app

ENV APP_HOME=$HOME/app

WORKDIR $APP_HOME
COPY package.json yarn.lock $APP_HOME/
RUN mkdir $APP_HOME/backend

COPY ./backend/package.json $APP_HOME/backend/
COPY ./backend/config $APP_HOME/backend/config

RUN yarn workspace backend install

COPY ./backend/ $APP_HOME/backend/

WORKDIR $APP_HOME/backend

RUN yarn build

RUN rm -rf node_modules
RUN yarn workspace backend install --prod

FROM node:14.18.1-alpine

RUN mkdir -p /app

COPY --from=0 /home/merrymaker/app/node_modules /app/node_modules
COPY --from=0 /home/merrymaker/app/backend/dist /app
COPY --from=0 /home/merrymaker/app/backend/package.json /app
COPY --from=0 /home/merrymaker/app/backend/config /app/config

RUN addgroup -S -g 992 merrymaker
RUN adduser -S -G merrymaker merrymaker

RUN chown -R merrymaker:merrymaker /app

USER merrymaker

WORKDIR /app
CMD ["node", "/app/app.js"]
