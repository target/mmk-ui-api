import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'

import { QueryBuilder } from 'objection'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import Scan, { Schema } from '../../../models/scans'
import { eagerLoad } from './handlers'
import ScanService from '../../../services/scan'

export default AsyncGet({
  tags: ['scans'],
  description: 'View Scan',
  parameters: [
    uuidParams,
    QueryParam({
      name: 'eager',
      description: 'Eager load Site or Source',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: ['sources', 'sites'],
        },
      },
    }),
  ],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: Schema,
            // TODO - missing optional eagers
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
      const { eager } = req.query as Record<string, string[]>
      res.locals.whereBuilder = (builder: QueryBuilder<Scan>) => {
        if (eager && Array.isArray(eager)) {
          eagerLoad(req.query.eager as string[], builder)
        }
      }
      next()
    },
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await ScanService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
