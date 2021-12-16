import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import AllowListService from '../../../services/allow_list'

export default AsyncDelete({
  tags: ['allow_list'],
  description: 'Delete Allow List',
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
      const deleted = await AllowListService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
})
