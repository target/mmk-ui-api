import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { validationErrorResponse } from '../../crud/schemas'
import { uuidParams } from '../../crud/schemas'
import { iocResponse } from './schemas'
import IocService from '../../../services/ioc'

export default AsyncGet({
  tags: ['iocs'],
  description: 'View IOC',
  parameters: [uuidParams],
  responses: {
    '200': iocResponse,
    '404': {
      description: 'Not Found',
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const record = await IocService.view(req.params.id)
      res.status(200).send(record)
      next()
    },
  ],
})
