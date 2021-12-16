import { ParamSchema, MediaSchema } from 'aejo'
import { Schema } from '../../../models/sources'
import { Schema as SecretSchema } from '../../../models/secrets'

export const sourceBody: MediaSchema = {
  description: 'Source Body',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          source: {
            type: 'object',
            properties: {
              name: Schema.name,
              value: Schema.value,
              secret_ids: {
                description: 'Array of associated Secret IDs',
                type: 'array',
                items: {
                  type: 'string',
                  format: 'uuid',
                },
              },
            },
            required: ['name', 'value'],
            additionalProperties: false,
          },
        },
        required: ['source'],
        additionalProperties: false,
      },
    },
  },
}

export const eagerResponse: { [prop: string]: ParamSchema } = {
  scans: {
    type: 'array',
    description: 'ID of related scans',
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
  secrets: {
    type: 'array',
    description: 'related secrets',
    items: {
      type: 'object',
      properties: {
        id: SecretSchema.id,
        name: SecretSchema.name,
        type: SecretSchema.type,
      },
    },
  },
}

export const sourceResponse: MediaSchema = {
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
