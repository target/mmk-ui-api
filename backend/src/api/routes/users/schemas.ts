import { MediaSchema } from 'aejo'
import { Schema } from '../../../models/users'

export const userSchema = {
  id: Schema.id,
  login: Schema.login,
  role: Schema.role,
  created_at: Schema.created_at,
  updated_at: Schema.updated_at,
}

export const userResponse: MediaSchema = {
  description: 'OK',
  content: {
    'application/json': {
      schema: {
        type: 'object',
        properties: userSchema,
      },
    },
  },
}
