import { Request, Response, NextFunction } from 'express'
import { AsyncPut } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { Schema } from '../../../models/secrets'
import { secretResponse } from './schemas'
import SecretService from '../../../services/secret'

export default AsyncPut({
  tags: ['secrets'],
  description: 'Update Secret',
  parameters: [uuidParams],
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
                type: Schema.type,
                value: Schema.value,
              },
              required: ['type', 'value'],
              additionalProperties: false,
            },
          },
          required: ['secret'],
          additionalProperties: false,
        },
      },
    },
  },
  responses: {
    '200': secretResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const updated = await SecretService.update(req.params.id, req.body.secret)
      res.status(200).send(updated)
      next()
    },
  ],
})
