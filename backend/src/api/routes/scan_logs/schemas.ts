import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/scan_logs'

export const scanLogResponse: MediaSchema = {
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
