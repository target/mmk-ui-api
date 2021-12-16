import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { config } from 'node-config-ts'

import OauthService from '../../../services/oauth'
import UserService from '../../../services/user'

export default AsyncGet({
  tags: ['auth'],
  description: 'Verify auth strategies are ready',
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              ready: {
                type: 'boolean',
              },
              strategy: {
                type: 'string',
                enum: ['local', 'oauth'],
              },
            },
          },
        },
      },
    },
  },
  middleware: [
    async (_req: Request, res: Response, next: NextFunction): Promise<void> => {
      if (config.auth.strategy === 'oauth') {
        res
          .status(200)
          .send({ strategy: 'oauth', ready: OauthService.client !== undefined })
      } else {
        const adminInst = await UserService.findOne({
          login: 'admin',
        })
        res
          .status(200)
          .send({ strategy: 'local', ready: adminInst !== undefined })
      }
      next()
    },
  ],
})
