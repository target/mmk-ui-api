import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { scanLogResponse } from './schemas'
import ScanLogService from '../../../services/scan_logs'

export default AsyncGet({
  tags: ['scan_logs'],
  description: 'View ScanLog',
  parameters: [uuidParams],
  responses: {
    '200': scanLogResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await ScanLogService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
