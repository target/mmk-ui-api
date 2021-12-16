import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import SourceService from '../../../services/source'

export default AsyncDelete({
  tags: ['sources'],
  description: 'Delete Source',
  parameters: [uuidParams],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const deleted = await SourceService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
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
      description: 'Not found',
    },
    '422': validationErrorResponse,
  },
})
