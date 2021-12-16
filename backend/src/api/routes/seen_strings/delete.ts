import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import SeenStringService from '../../../services/seen_string'

export default AsyncDelete({
  tags: ['seen_string'],
  description: 'Delete Seen String',
  parameters: [uuidParams],
  responses: {
    '200': {
      description: 'OK',
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
      const deleted = await SeenStringService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
})
