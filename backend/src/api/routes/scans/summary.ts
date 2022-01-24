import { Request, Response, NextFunction } from 'express'

import { AsyncGet } from 'aejo'
import { uuidParams } from '../alerts/schemas'
import ScanService from '../../../services/scan'

export default AsyncGet({
  tags: ['scans'],
  description: 'Scan Summary',
  parameters: [uuidParams],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { id } = req.params
      await ScanService.view(id)
      const summary = await ScanService.summary(id)
      res.status(200).json(summary)
      next()
    }
  ],
  responses: {
    200: {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              requests: {
                type: 'array',
                description: 'Number of times domain was requested',
                items: {
                  type: 'array',
                  example: ['www.foo.com', 5]
                }
              },
              totalAlerts: {
                description: 'Number of alerts',
                type: 'integer'
              },
              totalCookies: {
                description: 'Number of cookies set',
                type: 'integer'
              },
              totalErrors: {
                description: 'Number of script/page errors',
                type: 'integer'
              },
              totalFunc: {
                description: 'Number of function calls',
                type: 'integer'
              },
              totalReq: {
                description: 'Number of web requests',
                type: 'integer'
              }
            }
          }
        }
      }
    },
    404: {
      description: 'Not Found'
    }
  }
})
