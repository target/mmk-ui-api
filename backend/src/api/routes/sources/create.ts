import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import SourceService from '../../../services/source'
import { sourceBody, sourceResponse } from './schemas'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['sources'],
  description: 'Create Source',
  requestBody: sourceBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { source } = req.body
      const record = await SourceService.create(source)
      res.status(200).send(record)
      next()
    },
  ],
  responses: {
    '200': sourceResponse,
    '422': validationErrorResponse,
  },
})
