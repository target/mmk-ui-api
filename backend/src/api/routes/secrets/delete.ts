import { Request, Response, NextFunction } from 'express'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { AsyncDelete } from 'aejo'
import { ClientError } from '../../middleware/client-errors'
import SecretService from '../../../services/secret'

export default AsyncDelete({
  tags: ['secrets'],
  description: 'Delete Secret',
  parameters: [uuidParams],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              total: {
                type: 'integer',
              },
            },
          },
        },
      },
    },
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const active = await SecretService.isInUse(req.params.id)
      if (active) {
        throw new ClientError('Cannot delete active secret')
      }
      const deleted = await SecretService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
})
