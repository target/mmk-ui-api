import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import AlertService from '../../../services/alert'
import Alert from '../../../models/alerts'
import { validationErrorResponse } from '../../crud/schemas'

const selectable = Alert.selectAble() as string[]

export default AsyncGet({
  tags: ['alerts'],
  description: 'Fetches distinct column values from Alerts',
  parameters: [
    QueryParam({
      name: 'column',
      description: 'Distinct column',
      schema: {
        type: 'string',
        enum: selectable,
      },
    }),
  ],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'array',
            items: {
              type: 'object',
            },
          },
        },
      },
    },
    '422': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const result = await AlertService.distinct(req.query.column as string)
      res.status(200).send(result)
      next()
    },
  ],
})
