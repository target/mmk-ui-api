import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams } from './schemas'
import AlertService from '../../../services/alert'

export default AsyncDelete({
  tags: ['alerts'],
  description: 'Delete Alert',
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
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const result = AlertService.destroy(req.params.id)
      res.status(200).send(result)
      next()
    },
  ],
})
