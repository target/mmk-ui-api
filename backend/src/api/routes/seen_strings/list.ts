import { Response, Request, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import { QueryBuilder } from 'objection'
import { listHandler, ListQueryParams } from '../../crud/list'
import SeenString, { Schema } from '../../../models/seen_strings'

const selectable = SeenString.selectAble()

export default AsyncGet({
  tags: ['seen_string'],
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
            additionalProperties: false,
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      res.locals.whereBuilder = (builder: QueryBuilder<SeenString>) => {
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
    listHandler<SeenString>(SeenString, selectable),
  ],
})
