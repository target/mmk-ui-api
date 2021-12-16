import { MediaSchema, PathParam } from 'aejo'
import { Schema } from '../../../models/alerts'

export const uuidParams = PathParam({
  name: 'id',
  description: 'Alert ID',
  schema: {
    type: 'string',
    format: 'uuid',
  },
})

export const alertResponse: MediaSchema = {
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
