import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import AuthService from '../../../services/auth'
import { InvalidCreds } from '../../middleware/client-errors'

import { Schema } from '../../../models/users'

export default AsyncPost({
  tags: ['auth'],
  description: 'Local User Auth Login',
  requestBody: {
    description: 'User login credentials',
    content: {
      'application/json': {
        schema: {
          type: 'object',
          properties: {
            user: {
              type: 'object',
              properties: {
                login: Schema.login,
                password: Schema.password,
              },
              required: ['login', 'password'],
              additionalProperties: false,
            },
          },
          required: ['user'],
          additionalProperties: false,
        },
      },
    },
  },
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              login: Schema.login,
              role: Schema.role,
              created_at: Schema.created_at,
              updated_at: Schema.updated_at,
            },
          },
        },
      },
    },
    '403': {
      description: 'Invalid Login',
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const result = await AuthService.verifyLocalCreds(req.body.user)
      if (result.auth) {
        req.session.data = AuthService.buildSession(result.user)
        res.status(200).send(result.user)
        next()
      } else {
        throw new InvalidCreds('local', 'guest')
      }
    },
  ],
})
