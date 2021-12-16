import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/allow_list'

export const allowListBody: MediaSchema = {
  description: 'Allow List Object',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          allow_list: {
            type: 'object',
            properties: {
              type: Schema.type,
              key: Schema.key,
            },
            required: ['type', 'key'],
            additionalProperties: false,
          },
        },
        additionalProperties: false,
        required: ['allow_list'],
      },
    },
  },
}

export const allowListResponse: MediaSchema = {
  description: 'OK',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: Schema,
      },
    },
  },
}
