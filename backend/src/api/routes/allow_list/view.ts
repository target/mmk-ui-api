import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { allowListResponse } from './schemas'
import AllowListService from '../../../services/allow_list'

export default AsyncGet({
  tags: ['allow_list'],
  description: 'View Allow List',
  parameters: [uuidParams],
  responses: {
    '200': allowListResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await AllowListService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
