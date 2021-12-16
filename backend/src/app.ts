import express from 'express'
import connectRedis from 'connect-redis'
import expressSession from 'express-session'

import { Response, Request, NextFunction } from 'express'
import { config } from 'node-config-ts'
import app from './express-boot'
import https from 'https'
import fs from 'fs'
import { redisClient } from './repos/redis'

const RedisStore = connectRedis(expressSession)

import logger from './loaders/logger'

async function startServer() {
  const web = app({
    app: express(),
    middleware: expressSession({
      store: new RedisStore({ client: redisClient }),
      secret: config.session.secret,
      saveUninitialized: false,
      rolling: true,
      resave: false,
      cookie: {
        maxAge: config.session.maxAge,
      },
    }),
  })

  web.app.listen(config.port, () => {
    logger.info(`Listening on ${config.port}`)
  })

  if (config.transport.enabled) {
    const server = app({
      app: express(),
      middleware: expressSession({
        secret: config.session.secret,
        saveUninitialized: false,
        rolling: false,
        resave: false,
        unset: 'destroy',
      }),
      middlewareSession: (
        req: Request,
        _res: Response,
        next: NextFunction
      ): void => {
        req.session.data = {
          lanid: 'transport',
          firstName: 'transport',
          lastName: 'user',
          role: 'transport',
          email: 'mmk.transport@mydomain.com',
          isAuth: true,
        } as UserSession
        next()
      },
    })

    if (config.transport.mTLS) {
      try {
        const transportCert = fs.readFileSync(config.transport.cert)
        const transportKey = fs.readFileSync(config.transport.key)
        https
          .createServer(
            {
              cert: transportCert,
              key: transportKey,
            },
            server.app
          )
          .listen(config.transport.port)
      } catch (e) {
        logger.error(`Failed creating mTLS server`, e.message)
      }
    } else {
      server.app.listen(config.transport.port, () => {
        logger.info(`Transport server Listening on ${config.transport.port}`)
      })
    }
  }
}

startServer()
