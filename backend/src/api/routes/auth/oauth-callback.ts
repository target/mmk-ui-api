import { Request, Response, NextFunction } from 'express'
import { JwtDecode } from 'jwt-js-decode'
import { config } from 'node-config-ts'
import { AsyncGet, QueryParam } from 'aejo'

import OauthService from '../../../services/oauth'
import {
  UnauthorizedError,
  ForbiddenError,
} from '../../middleware/client-errors'

export default AsyncGet({
  tags: ['auth'],
  parameters: [
    QueryParam({
      name: 'code',
      description: 'Oauth callback code',
      required: true,
      schema: {
        type: 'string',
      },
    }),
  ],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      if (config.auth.strategy !== 'oauth') {
        res.status(404).send({ message: 'not enabled' })
        return next()
      }
      let idToken: JwtDecode
      const { nonce } = req.session.data
      try {
        idToken = await OauthService.tokenRequest(
          nonce,
          req.query.code as string
        )
      } catch (e) {
        throw new UnauthorizedError('oauth', e.message)
      }

      if (OauthService.oauthAuthorize(req.session, idToken)) {
        res.redirect(301, config.server.uri)
      } else {
        throw new ForbiddenError('oauth-callback', 'guest')
      }
    },
  ],
})
