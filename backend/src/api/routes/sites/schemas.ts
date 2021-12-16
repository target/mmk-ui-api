import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/sites'

export const siteResponse: MediaSchema = {
  description: 'Ok',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: Schema,
      },
    },
  },
}

export const siteBody: MediaSchema = {
  description: 'Site Object',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          site: {
            type: 'object',
            properties: {
              name: Schema.name,
              active: Schema.active,
              source_id: Schema.source_id,
              run_every_minutes: Schema.run_every_minutes,
            },
            required: ['name', 'active', 'source_id', 'run_every_minutes'],
            additionalProperties: false,
          },
        },
        required: ['site'],
        additionalProperties: false,
      },
    },
  },
}
