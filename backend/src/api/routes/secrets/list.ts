import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import { QueryBuilder } from 'objection'
import Secret, { Schema } from '../../../models/secrets'
import Source from '../../../models/sources'
import { listHandler, ListQueryParams } from '../../crud/list'

const selectable = Secret.selectAble()

const eagerLoad = (eagers: string[], builder: QueryBuilder<Secret>) => {
  if (eagers.includes('sources')) {
    builder.withGraphFetched('sources(selectID)').modifiers({
      selectID(builder: QueryBuilder<Source>) {
        builder.select('id')
      },
    })
  }
}

export default AsyncGet({
  tags: ['secrets'],
  description: 'List Secrets',
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
      description: 'Eager load related Source name',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: ['sources'],
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
              results: {
                type: 'array',
                items: {
                  type: 'object',
                  properties: {
                    ...Schema,
                    // eager loaded scan IDs
                    scans: {
                      type: 'array',
                      description: 'ID of related sources',
                      items: {
                        type: 'object',
                        properties: {
                          id: {
                            type: 'string',
                            format: 'uuid',
                          },
                        },
                      },
                    },
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
      const { eager } = req.query as Record<string, string[]>
      res.locals.whereBuilder = (builder: QueryBuilder<Secret>) => {
        if (eager && Array.isArray(req.query.eager)) {
          const eagers = req.query.eager as string[]
          eagerLoad(eagers, builder)
        }
      }
      next()
    },
    listHandler<Secret>(Secret, selectable),
  ],
})
