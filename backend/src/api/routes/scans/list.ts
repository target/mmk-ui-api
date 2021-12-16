import { Request, Response, NextFunction } from 'express'

import { QueryBuilder } from 'objection'
import { AsyncGet, QueryParam } from 'aejo'
import Scan, { Schema } from '../../../models/scans'
import { eagerLoad } from './handlers'
import { listHandler, ListQueryParams } from '../../crud/list'

const selectable = Scan.selectAble()

export default AsyncGet({
  tags: ['scans'],
  description: 'List Scans',
  parameters: [
    ...ListQueryParams,
    QueryParam({
      name: 'site_id',
      description: 'Filter by site_id',
      schema: {
        type: 'string',
        format: 'uuid',
      },
    }),
    QueryParam({
      name: 'eager',
      description: 'Eager load related Site name',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: ['sites', 'sources'],
        },
      },
    }),
    QueryParam({
      name: 'test',
      description: 'Filter test status',
      schema: {
        type: 'boolean',
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
              results: {
                type: 'array',
                items: {
                  type: 'object',
                  properties: {
                    ...Schema,
                  },
                },
              },
              total: {
                type: 'integer',
                description: 'Total number of results',
              },
            },
            additionalProperties: false,
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { site_id, eager, no_test } = req.query as Record<
        string,
        string | string[]
      >
      // filter on site_id
      res.locals.whereBuilder = (builder: QueryBuilder<Scan>) => {
        if (site_id) {
          builder.where('site_id', site_id)
        }
        if (eager && Array.isArray(eager)) {
          eagerLoad(eager as string[], builder)
        }
        if (no_test && no_test === 'true') {
          builder.whereNot('test', true)
        }
      }
      next()
    },
    listHandler<Scan>(Scan, selectable),
  ],
})
