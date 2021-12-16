import express, { Request, Response, NextFunction, Express } from 'express'

import ErrorMiddlewareHandler from './api/middleware/error-handler'
import ObjectionErrorHandler from './api/middleware/objection-errors'
import AejoErrorHandler from './api/middleware/aejo-errors'

import logger from './loaders/logger'
import routes from './api'
import httpEvent from './subscribers/http'
import { PathItem } from 'aejo'

function unknownErrorHandler(err: Error, req: Request, res: Response): boolean {
  httpEvent.emit('error', { req, res, err })
  logger.error(err.message)
  res.status(500).send({
    message: 'Unknown error occured',
    type: 'Unknown',
    data: {},
  })
  return true
}

type RequestHandler = (
  req: Request,
  res: Response,
  next: NextFunction
) => Express | void

function server(opts: {
  app: Express
  middleware?: RequestHandler
  middlewareSession?: RequestHandler
}): { app: Express; paths: PathItem } {
  const { app } = opts
  app.enable('trust proxy')

  if (opts.middleware) {
    app.use(opts.middleware)
  }

  app.use(express.json())

  // inject transport session values
  if (opts.middlewareSession) {
    app.set('middlewareSession', opts.middlewareSession)
    app.use((...args) => app.get('middlewareSession')(...args))
  }

  const apis = routes(app)

  app.use('/api', apis.router)

  // error handling
  app.use((err: Error, req: Request, res: Response, next: NextFunction) => {
    let handled = false
    try {
      handled =
        ErrorMiddlewareHandler(err, req, res) ||
        AejoErrorHandler(err, req, res) ||
        ObjectionErrorHandler(err, req, res) ||
        unknownErrorHandler(err, req, res)
    } catch (e) {
      handled = unknownErrorHandler(err, req, res)
    }
    if (!handled) {
      next()
    }
  })

  return { app, paths: apis.pathItem }
}

export default server
