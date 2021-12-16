import { AsyncDelete } from 'aejo'
import { Request, Response, NextFunction } from 'express'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import UserService from '../../../services/user'

export default AsyncDelete({
  tags: ['users'],
  description: 'Delete User',
  parameters: [uuidParams],
  responses: {
    '200': {
      description: 'OK',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              total: {
                type: 'integer',
              },
            },
          },
        },
      },
    },
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const deleted = await UserService.destroy(req.params.id)
      res.status(200).send({ total: deleted })
      next()
    },
  ],
})
