import { Request, Response, NextFunction } from 'express'
import { AsyncPut } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { allowListBody, allowListResponse } from './schemas'
import AllowListService from '../../../services/allow_list'

export default AsyncPut({
  tags: ['allow_list'],
  description: 'Update Allow List',
  parameters: [uuidParams],
  requestBody: allowListBody,
  responses: {
    '200': allowListResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const updated = await AllowListService.update(
        req.params.id,
        req.body.allow_list
      )
      res.status(200).send(updated)
      next()
    },
  ],
})
