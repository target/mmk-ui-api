import { Request, Response, NextFunction } from 'express'
import { AsyncPut } from 'aejo'
import { uuidParams, validationErrorResponse } from '../../crud/schemas'
import { iocBody, iocResponse } from './schemas'

import IocService from '../../../services/ioc'

export default AsyncPut({
  tags: ['iocs'],
  description: 'Update IOC',
  parameters: [uuidParams],
  requestBody: iocBody,
  responses: {
    '200': iocResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const updated = await IocService.update(req.params.id, req.body.ioc)
      res.status(200).send(updated)
      next()
    },
  ],
})
