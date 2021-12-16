import { Response, Request, NextFunction } from 'express'
import { AsyncGet } from 'aejo'

import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import SeenStringService from '../../../services/seen_string'
import { seenStringResponse } from './schemas'

export default AsyncGet({
  tags: ['seen_string'],
  description: 'View Seen String',
  parameters: [uuidParams],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await SeenStringService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
  responses: {
    '200': seenStringResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
})
