import { Request, Response } from 'express'
import { ScopeHandler, Security, AuthPathOp, Scope } from 'aejo'

import AuthService from '../../services/auth'
import { UserRole } from '../../models/users'
import HTTPSubscriber from '../../subscribers/http'
import { UnauthorizedError } from './client-errors'

interface UserAuth extends Request {
  user?: {
    session: Request['session']
  }
}

// Scope on Existing Session
export const UserAuth: ScopeHandler = (req: UserAuth): boolean =>
  AuthService.isAuth(req.session)

// Scope on Nonce (oauth init)
export const NonceAuth: ScopeHandler = (req: UserAuth): boolean =>
  req.session?.data?.nonce !== undefined

export const RoleAuth = (role: UserRole): ScopeHandler => (
  req: UserAuth
): boolean => {
  if (req.session?.data) {
    return AuthService.hasRole(req.session.data as UserSession, role)
  }
  throw new UnauthorizedError('auth', 'guest')
}

export const Authenticated: Security<'user'> = {
  name: 'authenticated',
  handler: (_req: Request, res: Response) => {
    res.status(401).send('Not Authenticated')
  },
  scopes: {
    user: UserAuth,
  },
  responses: {
    '401': {
      description: 'Not Authenticated',
    },
  },
}

export const OauthNonce: Security<'oauth'> = {
  name: 'OauthNonce',
  handler: (req: Request, res: Response) => {
    HTTPSubscriber.emit('event', {
      req,
      res,
      action: {
        message: 'oauth - Missing nonce',
        body: 'Session missing nonce in oauthCallback',
        level: 'warn',
      },
    })
    res.status(401).send('Not Authenticated')
  },
  scopes: {
    oauth: NonceAuth,
  },
  responses: {
    '401': {
      description: 'Not Authenticated',
    },
  },
}

export const Authorized: Security<'user' | 'admin' | 'transport'> = {
  name: 'authorized',
  handler: (_req: Request, res: Response) => {
    res.status(403).send('Forbidden')
  },
  scopes: {
    user: RoleAuth('user'),
    admin: RoleAuth('admin'),
    transport: RoleAuth('transport'),
  },
  responses: {
    '403': {
      description: 'Forbidden',
    },
  },
}

export const AuthScope = AuthPathOp(Scope(Authenticated, 'user'))
export const OauthScope = AuthPathOp(Scope(OauthNonce, 'oauth'))
