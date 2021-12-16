import { Request, Response, NextFunction } from 'express'
import { AsyncPut } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { siteBody, siteResponse } from './schemas'
import SiteService from '../../../services/site'

export default AsyncPut({
  tags: ['sites'],
  description: 'Update Site',
  parameters: [uuidParams],
  requestBody: siteBody,
  responses: {
    '200': siteResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const updated = await SiteService.update(req.params.id, req.body.site)
      res.status(200).send(updated)
      next()
    },
  ],
})
