import { Router, Express } from 'express'
import swaggerUI from 'swagger-ui-express'

import { Paths, Controller, PathItem, ajv } from 'aejo'
import sites from './routes/sites'
import auth from './routes/auth'
import allowList from './routes/allow_list'
import ioc from './routes/iocs'
import seenStrings from './routes/seen_strings'
import sources from './routes/sources'
import scans from './routes/scans'
import secrets from './routes/secrets'
import scanLogs from './routes/scan_logs'
import alerts from './routes/alerts'
import queues from './routes/queues'
import users from './routes/users'

import oas from './oas'

ajv.opts.coerceTypes = true

export default (express: Express): { router: Router; pathItem: PathItem } => {
  const app = Router()

  const pathItem = Paths(
    express,
    Controller({
      prefix: '/api/auth',
      route: auth,
    }),
    Controller({
      prefix: '/api/allow_list',
      route: allowList,
    }),
    Controller({
      prefix: '/api/alerts',
      route: alerts,
    }),
    Controller({
      prefix: '/api/iocs',
      route: ioc,
    }),
    Controller({
      prefix: '/api/scans',
      route: scans,
    }),
    Controller({
      prefix: '/api/scan_logs',
      route: scanLogs,
    }),
    Controller({
      prefix: '/api/sites',
      route: sites,
    }),
    Controller({
      prefix: '/api/users',
      route: users,
    }),
    Controller({
      prefix: '/api/queues',
      route: queues,
    }),
    Controller({
      prefix: '/api/secrets',
      route: secrets,
    }),
    Controller({
      prefix: '/api/seen_strings',
      route: seenStrings,
    }),
    Controller({
      prefix: '/api/sources',
      route: sources,
    })
  )

  const swaggerDoc = oas(pathItem)

  app.use('/api-docs', swaggerUI.serve)
  app.use('/api-docs', swaggerUI.setup(swaggerDoc))

  return { router: app, pathItem }
}
