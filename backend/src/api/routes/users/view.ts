import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { validationErrorResponse, uuidParams } from '../../crud/schemas'
import { userResponse } from './schemas'
import UserService from '../../../services/user'

export default AsyncGet({
  tags: ['users'],
  description: 'Get user',
  parameters: [uuidParams],
  responses: {
    '200': userResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await UserService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
