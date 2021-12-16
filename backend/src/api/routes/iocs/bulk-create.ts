import { Request, Response, NextFunction } from 'express'
import { AsyncPost } from 'aejo'
import { validationErrorResponse } from '../../crud/schemas'
import IocService from '../../../services/ioc'
import { IocType } from '../../../models/iocs'

export default AsyncPost({
  tags: ['iocs'],
  description: 'Bulk create IOCs',
  requestBody: {
    description: 'Bulk IOC create',
    content: {
      'application/json': {
        schema: {
          type: 'object',
          properties: {
            iocs: {
              type: 'object',
              properties: {
                values: {
                  type: 'array',
                  minItems: 1,
                  items: {
                    type: 'string',
                    format: 'regex',
                  },
                },
                type: {
                  type: 'string',
                  enum: ['fqdn', 'ip', 'literal'],
                },
                enabled: {
                  type: 'boolean',
                },
              },
              required: ['values', 'type', 'enabled'],
              additionalProperties: false,
            },
          },
          required: ['iocs'],
          additionalProperties: false,
        },
      },
    },
  },
  responses: {
    '200': {
      description: 'Ok',
    },
    '400': validationErrorResponse,
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const {
        values,
        type,
        enabled,
      }: {
        values: string[]
        type: IocType
        enabled: boolean
      } = req.body.iocs
      await IocService.bulkCreate({ values, type, enabled })
      res.status(200).send({ message: 'created' })
      next()
    },
  ],
})
