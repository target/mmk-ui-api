import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { config } from 'node-config-ts'

import { Oauth } from '../../../lib/oauth'
import OauthService from '../../../services/oauth'

export default AsyncGet({
  tags: ['auth'],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      if (config.auth.strategy !== 'oauth') {
        res.status(404).send({ message: 'not enabled' })
        return next()
      }
      const nonce = Oauth.generateNonce
      req.session.data = { nonce }
      res.redirect(OauthService.client.redirectURL(nonce))
    },
  ],
})
