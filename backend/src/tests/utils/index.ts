import express, { Response, Request, NextFunction, Express } from 'express'
import server from '../../express-boot'
import expressSession from 'express-session'
import { knex } from '../../models'

import { config } from 'node-config-ts'
import { PathItem } from 'aejo'

const memSession = expressSession({
  secret: config.session.secret,
  saveUninitialized: false,
  rolling: true,
  resave: false,
  unset: 'destroy',
  cookie: {
    maxAge: config.session.maxAge
  }
})

export function makeSession(
  session: UserSession = {}
): { app: Express; paths: PathItem } {
  return server({
    app: express(),
    middleware: memSession,
    middlewareSession: (req: Request, _res: Response, next: NextFunction) => {
      req.session.data = session
      next()
    }
  })
}

export const guestSession = (): { app: Express; paths: PathItem } => {
  return server({
    app: express(),
    middleware: memSession
  })
}

export async function resetDB(): Promise<void> {
  await knex.migrate.rollback({
    directory: './src/migrations',
    disableTransactions: true
  })
  await knex.migrate.latest({
    directory: './src/migrations',
    disableTransactions: true
  })
}
