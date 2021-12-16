import { Oauth } from '../lib/oauth'
import { Session } from 'express-session'
import { JwtDecode } from 'jwt-js-decode'
import querystring from 'querystring'
import { config } from 'node-config-ts'
import { UserRole } from '../models/users'

const oauthTokenRequest = (oauthClient: Oauth) => async (
  nonce: string,
  code: string
): Promise<JwtDecode> => {
  const token = await oauthClient.requestToken({
    nonce,
    code,
    grant_type: 'authorization_code',
    redirect_uri: config.oauth.redirectURL,
  })
  if (token.error) {
    throw new Error(`Oauth - ${token.error_description}`)
  }
  const idToken = oauthClient.decodeToken(token.id_token)
  if (querystring.unescape(idToken.payload.nonce) !== nonce) {
    throw new Error('Oauth nonce mismatch')
  }
  return idToken
}

const resolveAuthorization = (memberships: string[]): UserRole | null => {
  if (memberships.includes(config.authorizations.admin)) {
    return 'admin'
  } else if (memberships.includes(config.authorizations.transport)) {
    return 'transport'
  } else if (memberships.includes(config.authorizations.user)) {
    return 'user'
  }
  return null
}

/**
 * oauthAuthorize
 *   Decodes jwt token and compares `user` and `admin` authorizations
 *   against `memeberof` array
 */
const oauthAuthorize = (session: Session, token: JwtDecode): boolean => {
  if (!Array.isArray(token.payload.memberof)) {
    return false
  }
  if (token.payload.memberof.length === 0) {
    return false
  }
  const role = resolveAuthorization(token.payload.memberof)
  if (role === undefined) {
    return false
  }
  session.data = {
    role,
    lanid: token.payload.zid,
    firstName: token.payload.firstname,
    lastName: token.payload.lastname,
    email: token.payload.mail,
    isAuth: true,
    exp: token.payload.exp,
  } as UserSession
  return true
}

let client: Oauth
let tokenRequest: ReturnType<typeof oauthTokenRequest>

if (
  config.auth.strategy === 'oauth' &&
  // required values if using oauth
  config.oauth.authURL &&
  config.oauth.tokenURL &&
  config.oauth.clientID &&
  config.oauth.secret &&
  config.oauth.redirectURL
) {
  client = new Oauth({
    authURL: config.oauth.authURL,
    tokenURL: config.oauth.tokenURL,
    clientId: config.oauth.clientID,
    secret: config.oauth.secret,
    redirectURL: config.oauth.redirectURL,
    scope: config.oauth.scope,
  })
  tokenRequest = oauthTokenRequest(client)
}

export default {
  oauthTokenRequest,
  tokenRequest,
  client,
  resolveAuthorization,
  oauthAuthorize,
}
