import { Response, Request, NextFunction } from 'express'
import { cacheViewParams, cacheViewSchema } from '../../crud/cache'
import { AsyncGet } from 'aejo'
import SeenStringService from '../../../services/seen_string'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncGet({
  tags: ['seen_string'],
  description: 'Checks if record is found in cache',
  parameters: cacheViewParams,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { type, key } = req.query as Record<string, string>
      const hit = await SeenStringService.cached_view({ type, key })
      if (!hit.has) {
        const dbHit = await SeenStringService.findOne({
          type,
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
