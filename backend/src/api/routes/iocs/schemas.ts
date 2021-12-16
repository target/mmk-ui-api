import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/iocs'

export const iocBody: MediaSchema = {
  description: 'IOC Object',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          ioc: {
            type: 'object',
            properties: {
              value: Schema.value,
              type: Schema.type,
              enabled: Schema.enabled,
            },
            required: ['value', 'type', 'enabled'],
            additionalProperties: false,
          },
        },
        required: ['ioc'],
        additionalProperties: false,
      },
    },
  },
}

export const iocResponse: MediaSchema = {
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
