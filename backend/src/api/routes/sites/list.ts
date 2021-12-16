import { AsyncGet, QueryParam } from 'aejo'
import { listHandler, ListQueryParams } from '../../crud/list'
import Site, { Schema } from '../../../models/sites'

const selectable = Site.selectAble()

export default AsyncGet({
  tags: ['sites'],
  description: 'List Sites',
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
  ],
  middleware: [listHandler<Site>(Site, selectable)],
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
})
