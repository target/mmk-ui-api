import { QueryBuilder } from 'objection'
import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'

import Source from '../../../models/sources'
import { uuidParams } from '../../crud/schemas'
import { Schema } from '../../../models/sources'
import { eagerLoad } from './handlers'
import SourceService from '../../../services/source'

export default AsyncGet({
  tags: ['sources'],
  description: 'View Source',
  parameters: [
    uuidParams,
    QueryParam({
      name: 'eager',
      description: 'Eager load Scans or Secrets',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: ['scans', 'secrets'],
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
            properties: {
              ...Schema,
              ...eagerLoad,
            },
          },
        },
      },
    },
  },
  middleware: [
    // handler eager loaded related attributes
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      res.locals.whereBuilder = (builder: QueryBuilder<Source>) => {
        if (
          req.query.eager &&
          Array.isArray(req.query.eager) &&
          req.query.eager.length > 0
        ) {
          const eagers = req.query.eager as string[]
          eagerLoad(eagers, builder)
        }
      }
      next()
    },
    // view
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await SourceService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
