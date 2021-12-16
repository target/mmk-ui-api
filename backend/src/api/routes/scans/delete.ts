import { Request, Response, NextFunction } from 'express'
import { AsyncDelete } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'

import ScanService from '../../../services/scan'
import { ClientError } from '../../middleware/client-errors'

export default AsyncDelete({
  tags: ['scans'],
  description: 'Delete Scan',
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
      const { id } = req.params
      const active = await ScanService.isActive(id)
      if (active) {
        throw new ClientError('Cannot delete active scan')
      }
      const result = ScanService.destroy(id)
      res.status(200).send(result)
      next()
    },
  ],
})
