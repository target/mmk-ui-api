import { AsyncGet, QueryParam } from 'aejo'
import { Request, Response, NextFunction } from 'express'
import { QueryBuilder } from 'objection'
import { Alert } from '../../../models'
import { Site } from '../../../models'
import { Schema } from '../../../models/alerts'
import {
  listHandler,
  listResponseSchema,
  ListQueryParams,
} from '../../crud/list'

const selectable = Alert.selectAble() as string[]

const eagerLoad = (eagers: string[], builder: QueryBuilder<Alert>): void => {
  if (eagers.includes('site')) {
    builder.withGraphFetched('site(selectName)').modifiers({
      selectName(builder: QueryBuilder<Site>) {
        builder.select('name')
      },
    })
  }
}

export default AsyncGet({
  tags: ['alerts'],
  description: 'List Alerts',
  parameters: [
    ...ListQueryParams,
    QueryParam({
      name: 'scan_id',
      description: 'Filter by scan_id',
      schema: {
        type: 'string',
        format: 'uuid',
      },
    }),
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
          enum: ['site'],
        },
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
    QueryParam({
      name: 'rule',
      description: 'filter results based on rule type',
      schema: {
        type: 'array',
        items: {
          type: 'string',
          enum: Schema.rule.enum,
        },
      },
    }),
    QueryParam({
      name: 'search',
      description: 'full-text search on the Alert event',
      schema: {
        type: 'string',
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
              site: {
                type: 'object',
                nullable: true,
                description: 'Included when eager query is provided',
                properties: {
                  name: {
                    type: 'string',
                    description: 'Name of site',
                  },
                },
              },
            }),
            additionalProperties: false,
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      res.locals.whereBuilder = (builder: QueryBuilder<Alert>) => {
        const { rule, scan_id, site_id, eager } = req.query as Record<
          string,
          string | string[] | undefined
        >
        if (scan_id) {
          builder.where('scan_id', scan_id)
        }
        if (site_id) {
          builder.where('site_id', site_id)
        }
        if (rule) {
          builder.whereIn('rule', rule)
        }
        if (req.query.search && typeof req.query.search === 'string' && req.query.search.length > 0) {
          builder.whereRaw("to_tsvector('English', message) @@ ?::tsquery", [
            `${req.query.search.toLowerCase()}:*`,
          ])
        }

        if (eager && Array.isArray(eager)) {
          eagerLoad(eager, builder)
        }
      }
      next()
    },
    listHandler<Alert>(Alert, selectable),
  ],
})
