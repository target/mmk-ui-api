import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'

import IocService from '../../../services/ioc'

export default AsyncDelete({
  tags: ['iocs'],
  description: 'Delete IOC',
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
      const deleted = await IocService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
})
