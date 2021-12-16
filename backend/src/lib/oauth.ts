import querystring, { ParsedUrlQueryInput } from 'querystring'
import http from 'https'
import { IncomingMessage, ClientRequest } from 'http'
import crypto from 'crypto'

import { jwtDecode, JwtDecode } from 'jwt-js-decode'

interface AccessToken {
  /** Access Token - [RFC6749](https://tools.ietf.org/html/rfc6749#section-1.4) */
  access_token: string
  /** Access Token Type - [RFC6749](https://tools.ietf.org/html/rfc6749#section-7.1) */
  token_type: string
  /** Lifetime (seconds) of Access Token - [RFC6749](https://tools.ietf.org/html/rfc6749#section-4.2.2 */
  expires_in: number
  /** Access Token Scope - [RFC6749](https://tools.ietf.org/html/rfc6749#section-3.3) */
  scope: string
  id_token: string
  id_token_type: string
  /** encountered an error */
  error?: string
  error_description?: string
}

interface OauthOptions {
  /** Client Identifier - [RFC6749](https://tools.ietf.org/html/rfc6749#section-2.2) */
  clientId: string
  /** Client Password - [RFC6749](https://tools.ietf.org/html/rfc6749#section-2.3.1) */
  secret: string
  /** Authorization Endpoint - [RFC6749](https://tools.ietf.org/html/rfc6749#section-3.1) */
  authURL?: string
  /** Redirection Endpoint - [RFC6749](https://tools.ietf.org/html/rfc6749#section-3.1.2) */
  redirectURL?: string
  /** Token Endpoint - [RFC6749](https://tools.ietf.org/html/rfc6749#section-3.2) */
  tokenURL: string
  /** Access Token Scope - [RFC6749](https://tools.ietf.org/html/rfc6749#section-3.3) */
  scope: string
}

interface OauthRequest extends ParsedUrlQueryInput {
  nonce?: string
  grant_type: string
  redirect_uri?: string
}

export class Oauth {
  authURL: URL
  tokenHost: string
  tokenPath: string
  basicAuth: string
  constructor(private options: OauthOptions) {
    if (this.options.authURL) {
      this.authURL = new URL(this.options.authURL)
    }
    if (this.options.tokenURL) {
      const tokenURL = new URL(this.options.tokenURL)
      this.tokenHost = tokenURL.hostname
      this.tokenPath = tokenURL.pathname
    }
    this.basicAuth = `${this.options.clientId}:${this.options.secret}`
  }

  /**
   * generateNonce
   *  Generates secure nonce to verify the request has never been made previously
   */
  static get generateNonce(): string {
    return crypto.randomBytes(16).toString('base64')
  }

  /**
   * redirectURL
   *  Generates the Redirection Endpoint URL
   */
  public redirectURL(nonce: string): string {
    const params = new URLSearchParams({
      nonce,
      client_id: this.options.clientId,
      response_type: 'code',
      scope: this.options.scope,
      redirect_uri: this.options.redirectURL,
    })
    this.authURL.search = params.toString()
    return this.authURL.toString()
  }

  /**
   * requestToken
   *  Performs an Access Token Request - https://tools.ietf.org/html/rfc6749#section-4.1.3
   *
   *  Parses and returns a AccessToken
   */
  public async requestToken(
    options: OauthRequest
  ): Promise<AccessToken> {
    const payload = querystring.stringify(options)
    return new Promise((resolve, reject) => {
      let data = ''
      const treq = this.tokenClientRequest(payload, (res) => {
        res.setEncoding('utf8')
        res.on('data', (chunk) => (data += chunk))
        res.on('end', () => {
          try {
            resolve(JSON.parse(data))
          } catch (e) {
            reject(e)
          }
        })
        res.on('error', reject)
      })
      treq.on('error', reject)
      treq.write(payload)
      treq.end()
    })
  }

  /**
   * decodeToken
   *  Decodes a JWT encoded token
   */
  public decodeToken(token: string): JwtDecode {
    return jwtDecode(token)
  }

  /**
   * tokenClientRequest
   *  Creates an HTTP request used in the Token Access Request step
   */
  private tokenClientRequest(
    payload: string,
    callback: (res: IncomingMessage) => void
  ): ClientRequest {
    return http.request(
      {
        hostname: this.tokenHost,
        port: 443,
        path: this.tokenPath,
        method: 'POST',
        auth: this.basicAuth,
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/x-www-form-urlencoded',
          'Content-Length': Buffer.byteLength(payload),
        },
      },
      callback
    )
  }
}
