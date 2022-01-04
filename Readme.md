# Merry Maker 2.0

Merry Maker is a fully scalable tool to detect the presence of digital skimmers.

## Features

- [Puppeteer](https://pptr.dev/) scripts to simulate user interactions
- Yara rules for static analysis
- Hooks native JavaScript function calls for detection and attribution
- Near real-time browser event detection and alerting
- Distributed event scanning (rule engine)
- Role based UI with local and OAuth2 authentication options

## Full Stack Demo

```
# Start all the services
docker compose -f docker-compose.all.yml up
```

Navigate to `http://localhost:8080` to begin.

## Requirements

- docker
- node v14.18.1

## Setup

### Docker Stack

Includes `postgres`, `redis` and a `testRedis` instance

```
# from ./
docker compose up -d
```

### Backend

API service for the `frontend` and `scanner`

DB Migration

```
# from ./backend
yarn migrate
```

```
# from ./backend
yarn install

yarn start
```

Testing

```
yarn test
```

Uses nodemon to auto reload on change. Listens on two separate HTTP ports (UI and transport)

### Frontend

Vue dev server for developing the frontend. Run `backend` prior to starting this service

```
# from ./frontend
yarn install
yarn serve
```

### Jobs

Main scheduler for running scans, purging old data, and misc cron jobs

```
# from ./backend
yarn jobs
```

### Scanner

Rules runner for processing browser events emitted by `jsscope`

```
# from ./scanner
yarn install
yarn start
```

Testing

```
yarn test
```

### Optional Auth Strategy

#### OAuth2

```
export MMK_AUTH_STRATEGY=oauth
export MMK_OAUTH_AUTH_URL=http://oauth-server/auth/oauth/v2/authorize
export MMK_OAUTH_TOKEN_URL=https://oauth-server/auth/oauth/v2/token
export MMK_OAUTH_CLIENT_ID=client_id
export MMK_OAUTH_SECRET=<oauth-secret>
export MMK_OAUTH_REDIRECT_URL=http://localhost:8080/api/auth/login
export MMK_OAUTH_SCOPE=openid profile email
```

---

```
Copyright (c) 2021 Target Brands, Inc.
```
