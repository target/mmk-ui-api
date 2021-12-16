import { MediaSchema, PathParam, ParamSchema } from 'aejo'

export const uuidParams = PathParam({
  name: 'id',
  description: 'Record ID',
  schema: {
    type: 'string',
    format: 'uuid',
  },
})

export const validationContext: ParamSchema = {
  type: 'object',
  properties: {
    keyword: {
      type: 'string',
      enum: ['format'],
    },
    dataPath: {
      type: 'string',
    },
    schemaPath: {
      type: 'string',
    },
    params: {
      type: 'object',
      properties: {
        format: {
          type: 'string',
        },
      },
    },
    message: {
      type: 'string',
    },
  },
}

export const validationErrorResponse: MediaSchema = {
  description: 'ValidationError',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: {
          name: {
            type: 'string',
            enum: ['ValidationError'],
          },
          context: {
            type: 'array',
            items: validationContext,
          },
        },
      },
    },
  },
}

export const uuidFormat =
  '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
