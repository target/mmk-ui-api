import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import ScanLog from '../../../models/scan_logs'
import ScanLogService from '../../../services/scan_logs'

const selectable = ScanLog.selectAble() as string[]

export default AsyncGet({
  tags: ['scan_logs'],
  description: 'Fetches distinct column values from a single ScanLog',
  parameters: [
    uuidParams,
    QueryParam({
      name: 'column',
      required: true,
      description: 'Distinct column',
      schema: {
        type: 'string',
        enum: selectable,
      },
    }),
  ],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'array',
            items: {
              type: 'object',
            },
          },
        },
      },
    },
    '422': validationErrorResponse,
    '404': {
      description: 'Not Found',
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const result = await ScanLogService.distinct(req.query.column as string, {
        scan_id: req.params.id,
      })
      res.status(200).send(result)
      next()
    },
  ],
})
