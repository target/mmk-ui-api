import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import { AllowList } from '../../../models'
import { Schema } from '../../../models/allow_list'
import {
  listHandler,
  listResponseSchema,
  ListQueryParams,
} from '../../crud/list'
import { QueryBuilder } from 'objection'

const selectable = AllowList.selectAble()

export default AsyncGet({
  tags: ['allow_list'],
  description: 'List records',
  parameters: [
    ...ListQueryParams,
    QueryParam({
      name: 'key',
      description: 'filter on keys using a regular expression',
      schema: {
        type: 'string',
      },
    }),
    QueryParam({
      name: 'type',
      description: 'filter on type',
      schema: {
        type: 'string',
        enum: [
          'fqdn',
          'ip',
          'literal',
          'ioc-payload-domain',
          'google-analytics',
        ],
      },
    }),
    QueryParam({
      name: 'fields',
      description: 'Select fields from results',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: selectable,
        },
      },
    }),
  ],
  responses: {
    '200': {
      description: 'OK',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: listResponseSchema(Schema),
            additionalProperties: false,
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      res.locals.whereBuilder = (builder: QueryBuilder<AllowList>) => {
        if (req.query.type && typeof req.query.type === 'string') {
          builder.where('type', req.query.type)
        }
        // use `~` for all matches (regex)
        if (req.query.key && typeof req.query.key === 'string') {
          builder.whereRaw('? ~ key', [req.query.key])
        }
      }
      next()
    },
    listHandler<AllowList>(AllowList, selectable),
  ],
})
