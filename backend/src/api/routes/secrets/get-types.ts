import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { config } from 'node-config-ts'

const types = [
  'manual',
  ...(config.quantumTunnel.enabled === 'true' ? ['qt'] : []),
]

export default AsyncGet({
  tags: ['secrets'],
  description: 'Get Secret Types',
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              types: {
                type: 'array',
                items: {
                  type: 'string',
                  enum: types,
                },
              },
            },
          },
        },
      },
    },
  },
  middleware: [
    async (_req: Request, res: Response, next: NextFunction) => {
      res.status(200).send({ types })
      next()
    },
  ],
})
