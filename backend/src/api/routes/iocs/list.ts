import { Request, Response, NextFunction } from 'express'
import { QueryBuilder } from 'objection'
import { AsyncGet, QueryParam } from 'aejo'
import {
  listHandler,
  listResponseSchema,
  ListQueryParams,
} from '../../crud/list'
import Ioc, { Schema } from '../../../models/iocs'

const selectable = Ioc.selectAble()

export default AsyncGet({
  tags: ['iocs'],
  description: 'List IOCs',
  parameters: [
    ...ListQueryParams,
    QueryParam({
      name: 'type',
      description: 'filter on type',
      schema: {
        type: 'string',
        enum: ['fqdn', 'ip', 'literal'],
      },
    }),
    QueryParam({
      name: 'value',
      description: 'filter on values using a regular expression',
      schema: {
        type: 'string',
        format: 'regex',
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
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: listResponseSchema(Schema),
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      res.locals.whereBuilder = (builder: QueryBuilder<Ioc>) => {
        if (req.query.type && typeof req.query.type === 'string') {
          builder.where('type', req.query.type)
        }
        if (req.query.value && typeof req.query.value === 'string') {
          builder.whereRaw('? ~ value', [req.query.value])
        }
      }
      next()
    },
    listHandler<Ioc>(Ioc, selectable),
  ],
})
