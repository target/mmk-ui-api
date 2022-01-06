import { Request, Response, NextFunction } from 'express'
import { AsyncGet, QueryParam } from 'aejo'
import SeenStringService from '../../../services/seen_string'
import SeenString from '../../../models/seen_strings'
import { validationErrorResponse } from '../../crud/schemas'

const selectable = SeenString.selectAble() as string[]

export default AsyncGet({
  tags: ['seen_string'],
  description: 'Fetches distinct column values from Seen Strings',
  parameters: [
    QueryParam({
      name: 'column',
      description: 'Distinct column',
      schema: {
        type: 'string',
        enum: selectable
      }
    })
  ],
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'array',
            items: {
              type: 'object'
            }
          }
        }
      }
    },
    '422': validationErrorResponse
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const result = await SeenStringService.distinct(
        req.query.column as string
      )
      res.status(200).send(result)
      next()
    }
  ]
})
