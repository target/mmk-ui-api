import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/seen_strings'

export const seenStringBody: MediaSchema = {
  description: 'Seen String Object',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          seen_string: {
            type: 'object',
            properties: {
              type: Schema.type,
              key: Schema.key,
            },
            required: ['type', 'key'],
            additionalProperties: false,
          },
        },
        required: ['seen_string'],
        additionalProperties: false,
      },
    },
  },
}

export const seenStringResponse: MediaSchema = {
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
