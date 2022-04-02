import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { cacheViewParams, cacheViewSchema } from '../../crud/cache'
import { validationErrorResponse } from '../../crud/schemas'
import AllowListService from '../../../services/allow_list'
import { AllowListType } from '../../../models/allow_list'

export default AsyncGet({
  tags: ['allow_list'],
  description: 'Checks if record is found in cache',
  parameters: cacheViewParams,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { type, key } = req.query as Record<string, string>
      const hit = await AllowListService.cached_view({ type, key })
      // if not in cache, check the DB
      if (!hit.has) {
        const dbHit = await AllowListService.findOne({
          type: typeof AllowListType,
          key,
        })
        if (dbHit) {
          hit.store = 'database'
          hit.has = true
        }
      }
      res.status(200).send(hit)
      next()
    },
  ],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: cacheViewSchema,
        },
      },
    },
    '422': validationErrorResponse,
  },
})
