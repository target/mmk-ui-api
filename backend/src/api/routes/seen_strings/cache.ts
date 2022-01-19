import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { seenStringBody } from './schemas'
import { cacheViewSchema } from '../../crud/cache'
import SeenStringService from '../../../services/seen_string'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['seen_string'],
  description: 'Read-through cache',
  requestBody: seenStringBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { type, key } = req.body.seen_string as Record<string, string>
      const hit = await SeenStringService.cached_view({ type, key })
      if (!hit.has) {
        const dbHit = await SeenStringService.findOne({
          type,
          key,
        })
        if (dbHit) {
          await SeenStringService.update(dbHit.id, { last_cached: new Date() })
          hit.store = 'database'
          hit.has = true
        } else {
          await SeenStringService.create({
            type,
            key,
          })
          await SeenStringService.cached_write_view({ key, type }, 'database')
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
