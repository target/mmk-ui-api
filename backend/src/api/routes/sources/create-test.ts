import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import SourceService from '../../../services/source'
import ScanService from '../../../services/scan'
import { sourceBody } from './schemas'
import { getScannerQueue } from '../../../lib/queues'
import Queue from 'bull'
import { validationErrorResponse } from '../../crud/schemas'

let scannerQueue: Queue.Queue
;(async () => {
  scannerQueue = await getScannerQueue()
})()

export default AsyncPost({
  tags: ['sources'],
  description: 'Create temporary test source',
  requestBody: sourceBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { source } = req.body
      const tmp = await SourceService.create({
        ...source,
        test: true,
        name: `tmp${new Date().valueOf()}`,
      })
      const scheduledJob = await ScanService.schedule(scannerQueue, {
        source: tmp,
        test: true,
      })
      res.status(200).send({
        scan_id: scheduledJob.scan.id,
        source_id: tmp.id,
      })
      next()
    },
  ],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              scan_id: {
                description: 'Scheduled Scan',
                type: 'string',
                format: 'uuidv4',
              },
              source_id: {
                description: 'Temporary Source ID',
                type: 'string',
                format: 'uuidv4',
              },
            },
          },
        },
      },
    },
    '422': validationErrorResponse,
  },
})
