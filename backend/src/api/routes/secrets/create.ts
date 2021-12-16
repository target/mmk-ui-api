import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { Schema } from '../../../models/secrets'
import SecretService from '../../../services/secret'
import { secretResponse } from './schemas'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['secrets'],
  description: 'Create Secret',
  requestBody: {
    description: 'Secret Object',
    content: {
      'application/json': {
        schema: {
          type: 'object',
          properties: {
            secret: {
              type: 'object',
              properties: {
                name: Schema.name,
                type: Schema.type,
                value: Schema.value,
              },
              required: ['name', 'type', 'value'],
              additionalProperties: false,
            },
          },
          required: ['secret'],
          additionalProperties: false,
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await SecretService.create(req.body.secret)
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': secretResponse,
    '422': validationErrorResponse,
  },
})
