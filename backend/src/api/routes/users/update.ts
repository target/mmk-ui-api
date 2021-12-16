import { Request, Response, NextFunction } from 'express'
import { AsyncPut } from 'aejo'
import { uuidParams, validationErrorResponse } from '../..//crud/schemas'
import { Schema } from '../../../models/users'
import { userResponse } from './schemas'
import UserService from '../../../services/user'

export default AsyncPut({
  tags: ['users'],
  description: 'Update User',
  parameters: [uuidParams],
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
              required: ['login', 'role'],
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
    '200': userResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const updated = await UserService.update(req.params.id, req.body.user)
      res.status(200).send(updated)
      next()
    },
  ],
})
