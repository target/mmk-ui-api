import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import UserService from '../../../services/user'
import AuthService from '../../../services/auth'
import { UnauthorizedError } from '../../middleware/client-errors'
import { userResponse } from './schemas'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['users'],
  description: 'First-run local admin create endpoint',
  requestBody: {
    description: 'Admin Password',
    content: {
      'application/json': {
        schema: {
          type: 'object',
          properties: {
            password: {
              type: 'string',
              minLength: 8,
              maxLength: 32,
            },
          },
          required: ['password'],
          additionalProperties: false,
        },
      },
    },
  },
  middleware: [
    async (
      _req: Request,
      _res: Response,
      next: NextFunction
    ): Promise<void> => {
      const hasAdmin = await UserService.findOne({ login: 'admin' })
      if (hasAdmin !== undefined) {
        throw new UnauthorizedError('create_admin', 'guest')
      }
      return next()
    },
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await UserService.create({
        login: 'admin',
        role: 'admin',
        password: req.body.password,
      })
      req.session.data = AuthService.buildSession(created)
      delete created.password
      delete created.password_hash
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': userResponse,
    '422': validationErrorResponse,
  },
})
