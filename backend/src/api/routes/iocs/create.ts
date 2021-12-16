import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { iocBody, iocResponse } from './schemas'
import IocService from '../../../services/ioc'
import { validationErrorResponse } from '../../crud/schemas'

export default AsyncPost({
  tags: ['iocs'],
  description: 'Create IOC',
  requestBody: iocBody,
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const created = await IocService.create(req.body.ioc)
      res.status(200).send(created)
      next()
    },
  ],
  responses: {
    '200': iocResponse,
    '422': validationErrorResponse,
  },
})
