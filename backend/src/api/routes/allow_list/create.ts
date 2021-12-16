import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { allowListBody, allowListResponse } from './schemas'
import AllowListService from '../../../services/allow_list'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['allow_list'],
  description: 'Create Allow List',
  requestBody: allowListBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await AllowListService.create(req.body.allow_list)
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': allowListResponse,
    '422': validationErrorResponse,
  },
})
