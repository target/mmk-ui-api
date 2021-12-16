import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { cacheViewParams, cacheViewSchema } from '../../crud/cache'
import { validationErrorResponse } from '../../crud/schemas'
import IocService from '../../../services/ioc'
import { IocType } from '../../../models/iocs'

export default AsyncGet({
  tags: ['iocs'],
  description: 'Checks if record is found in cache',
  parameters: cacheViewParams,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { type, key } = req.query as Record<string, string>
      const hit = await IocService.cached_view({ type, key })
      if (!hit.has) {
        const dbHit = await IocService.findOne({
          type: type as IocType,
          value: key,
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
    '400': validationErrorResponse,
  },
})
