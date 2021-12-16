import { NextFunction, Request, Response } from 'express'
import { AsyncPost } from 'aejo'
import { seenStringBody, seenStringResponse } from './schemas'
import SeenStringService from '../../../services/seen_string'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['seen_string'],
  description: 'Create Seen String',
  requestBody: seenStringBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await SeenStringService.create(req.body.seen_string)
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': seenStringResponse,
    '422': validationErrorResponse,
  },
})
