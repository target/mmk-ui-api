import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/secrets'

export const secretResponse: MediaSchema = {
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
