import { Request, Response, NextFunction } from 'express'
import { AsyncPut } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { seenStringBody, seenStringResponse } from './schemas'
import SeenStringService from '../../../services/seen_string'

export default AsyncPut({
  tags: ['seen_string'],
  description: 'Update Seen String',
  parameters: [uuidParams],
  requestBody: seenStringBody,
  responses: {
    '200': seenStringResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const updated = await SeenStringService.update(
        req.params.id,
        req.body.seen_string
      )
      res.status(200).send(updated)
      next()
    },
  ],
})
