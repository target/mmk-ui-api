import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { validationErrorResponse } from '../../crud/schemas'
import AlertService from '../../../services/alert'
import { alertResponse, uuidParams } from './schemas'

export default AsyncGet({
  tags: ['alerts'],
  description: 'View Alert',
  parameters: [uuidParams],
  responses: {
    '200': alertResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await AlertService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
