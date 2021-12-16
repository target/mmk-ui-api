import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import { QueryBuilder } from 'objection'
import Source, { Schema } from '../../../models/sources'
import {
  listHandler,
  listResponseSchema,
  ListQueryParams,
} from '../..//crud/list'
import { eagerResponse } from './schemas'
import { eagerLoad } from './handlers'

const selectable = Source.selectAble()

export default AsyncGet({
  tags: ['sources'],
  description: 'List Sources',
  parameters: [
    ...ListQueryParams,
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
    QueryParam({
      name: 'eager',
      description: 'Eager load related Scan IDs or Secrets',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: ['scans', 'secrets'],
        },
      },
    }),
    QueryParam({
      name: 'no_test',
      description: 'Exclude test scans',
      schema: {
        type: 'string',
        enum: ['true'],
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
            properties: listResponseSchema({
              ...Schema,
              ...eagerResponse,
            }),
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      // includes option for joining on source or scan
      res.locals.selectable = []
      if (
        req.query.fields &&
        Array.isArray(req.query.fields) &&
        req.query.fields.length > 0
      ) {
        res.locals.selectable = req.query.fields
      }

      res.locals.whereBuilder = (builder: QueryBuilder<Source>) => {
        if (
          req.query.eager &&
          Array.isArray(req.query.eager) &&
          req.query.eager.length > 0
        ) {
          const eagers = req.query.eager as string[]
          // eager load sites
          eagerLoad(eagers, builder)
        }

        if (req.query.no_test && req.query.no_test === 'true') {
          builder.whereNot('test', true)
        }
      }
      next()
    },
    listHandler<Source>(Source, selectable),
  ],
})
