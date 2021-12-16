import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import SiteService from '../../../services/site'
import { validationErrorResponse } from '../../crud/schemas'
import { siteBody, siteResponse } from './schemas'

export default AsyncPost({
  tags: ['sites'],
  description: 'Create Site',
  requestBody: siteBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await SiteService.create(req.body.site)
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': siteResponse,
    '422': validationErrorResponse,
  },
})
