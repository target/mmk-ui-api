import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import { QueryBuilder } from 'objection'
import { listHandler, ListQueryParams } from '../../crud/list'
import { Schema } from '../../../models/scan_logs'
import { ScanLog } from '../../../models'

const selectable = ScanLog.selectAble() as string[]

export default AsyncGet({
  tags: ['scan_logs'],
  description: 'List Scan Logs',
  parameters: [
    QueryParam({
      name: 'scan_id',
      description: 'filter on scan_id',
      schema: {
        type: 'string',
        format: 'uuid',
      },
    }),
    QueryParam({
      name: 'search',
      description: 'full-text search on the ScanLog event',
      schema: {
        type: 'string',
      },
    }),
    QueryParam({
      name: 'from',
      description: 'date filter using created_at > `from`',
      schema: {
        type: 'string',
        format: 'date-time',
      },
    }),
    QueryParam({
      name: 'entry',
      description: 'filter results based on entry type',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: Schema.entry.enum,
        },
      },
    }),
    ...ListQueryParams,
  ],
  responses: {
    '200': {
      description: 'OK',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              results: {
                type: 'array',
                items: {
                  type: 'object',
                  properties: Schema,
                },
              },
              total: {
                type: 'integer',
                description: 'Total number of results',
              },
            },
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      // filter on scan_id
      res.locals.whereBuilder = (builder: QueryBuilder<ScanLog>) => {
        if (req.query.scan_id && typeof req.query.scan_id === 'string') {
          builder.where('scan_id', req.query.scan_id)
        }

        if (
          req.query.search &&
          typeof req.query.search === 'string' &&
          req.query.search.length > 0
        ) {
          builder.whereRaw("to_tsvector('English', event) @@ ?::tsquery", [
            `${req.query.search.toLowerCase()}:*`,
          ])
        }
        // allow filtering on created_at after a given date
        if (req.query.from && typeof req.query.from === 'string') {
          builder.whereRaw('created_at > ?', [req.query.from])
        }

        if (
          req.query.entry &&
          Array.isArray(req.query.entry) &&
          req.query.entry.length
        ) {
          builder.whereIn('entry', req.query.entry as string[])
        }
      }
      next()
    },
    listHandler<ScanLog>(ScanLog, selectable),
  ],
})
