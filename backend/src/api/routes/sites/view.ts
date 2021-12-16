import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { siteResponse } from './schemas'
import SiteService from '../../../services/site'

export default AsyncGet({
  tags: ['sites'],
  description: 'View Site',
  parameters: [uuidParams],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await SiteService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
  responses: {
    '200': siteResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
})
