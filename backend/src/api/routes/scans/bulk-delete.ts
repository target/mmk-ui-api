import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { validationErrorResponse } from '../../crud/schemas'

import ScanService from '../../../services/scan'
import { ClientError } from '../..//middleware/client-errors'

export default AsyncPost({
  tags: ['scans'],
  description: 'Bulk Delete Scan',
  requestBody: {
    description: 'Bulk Delete Scan Request',
    content: {
      'application/json': {
        schema: {
          type: 'object',
          properties: {
            scans: {
              type: 'object',
              properties: {
                ids: {
                  type: 'array',
                  items: {
                    type: 'string',
                    format: 'uuid',
                  },
                },
              },
              required: ['ids'],

              additionalProperties: false,
            },
          },
          required: ['scans'],
          additionalProperties: false,
        },
      },
    },
  },
  responses: {
    '200': {
      description: 'Ok',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { ids } = req.body.scans as Record<string, string[]>
      if (ids && Array.isArray(ids)) {
        const hasActives = await ScanService.isBulkActive(ids)
        if (hasActives) {
          throw new ClientError('Cannot delete active scans')
        }
        await ScanService.bulkDelete(ids)
        res.status(200).send({ message: 'ok' })
      }
      next()
    },
  ],
})
