import { Request, Response, NextFunction } from 'express'
import { validationErrorResponse } from '../../crud/schemas'
import { AsyncGet } from 'aejo'
import { secretResponse } from './schemas'
import SecretService from '../../../services/secret'

export default AsyncGet({
  tags: ['secrets'],
  description: 'View Secret',
  responses: {
    '200': secretResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await SecretService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
