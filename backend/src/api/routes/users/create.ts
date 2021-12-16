import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { validationErrorResponse } from '../../crud/schemas'
import { userResponse } from './schemas'
import { Schema } from '../../../models/users'
import UserService from '../../../services/user'

export default AsyncPost({
  tags: ['users'],
  description: 'Create User',
  requestBody: {
    description: 'User Object',
    content: {
      'application/json': {
        schema: {
          type: 'object',
          properties: {
            user: {
              type: 'object',
              properties: {
                login: Schema.login,
                role: Schema.role,
                password: Schema.password,
              },
              required: ['login', 'role', 'password'],
              additionalProperties: false,
            },
          },
          required: ['user'],
          additionalProperties: false,
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await UserService.create(req.body.user)
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': userResponse,
    '422': validationErrorResponse,
  },
})
