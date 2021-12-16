import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams } from '../../crud/schemas'
import SiteService from '../../../services/site'

export default AsyncDelete({
  tags: ['sites'],
  description: 'Delte Site',
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
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const deleted = await SiteService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
})
