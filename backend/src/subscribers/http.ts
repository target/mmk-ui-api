import { Request, Response } from 'express'
import events from 'events'
import crypto from 'crypto'
import logger from '../loaders/logger'

type EventAction = {
  message: string
  body: unknown
  level: 'info' | 'warn' | 'debug' | 'error'
}
interface RequestContext {
  http: RequestHttpContext
  user: RequestUserContext
}

interface HttpServerRequest {
  method: string
  schema: string
  host: string
  headers: Record<string, unknown>
}

interface HttpServerResponse {
  request_id: string
  status: number
  time_ms: number
  headers: Record<string, unknown>
}

type RequestHttpContext = {
  method: string
  path: string
  remote_host: string
  request_id: string
}

type RequestUserContext = {
  lanid: string
  user: string
  email: string
  role: string
}

export class HTTPSubscriber extends events.EventEmitter {
  constructor() {
    super()
    this.on('request', HTTPSubscriber.onRequest)
    this.on('event', HTTPSubscriber.onEvent)
    this.on('error', HTTPSubscriber.onError)
    this.on('clientWarning', HTTPSubscriber.onClientWarning)
  }

  public static onRequest(evt: { req: Request; res: Response }): void {
    const startHrTime = process.hrtime()
    const context = HTTPSubscriber.getContext(evt)
    evt.res.locals.request_id = context.http.request_id
    const headers = HTTPSubscriber.sanitizeHeaders(evt.req.headers)
    const httpServerRequest = HTTPSubscriber.getHttpRequest(evt.req, headers)
    logger.info(
      `${evt.req.method} ${evt.req.url} for ${context.http.remote_host}`,
      {
        context,
        event: { http_server_request: httpServerRequest },
      }
    )

    evt.res.on('finish', () => {
      const elapsedHrTime = process.hrtime(startHrTime)
      const elapsedHrTimeInMs = elapsedHrTime[0] * 1000 + elapsedHrTime[1] / 1e6
      const httpServerResponse = HTTPSubscriber.getHttpResponse(
        evt.res,
        elapsedHrTimeInMs
      )
      logger.info(
        `Sent ${evt.res.statusCode} in ${elapsedHrTimeInMs.toLocaleString()}ms`,
        {
          context,
          event: { http_server_response: httpServerResponse },
        }
      )
    })
  }

  public static onEvent(evt: {
    req: Request
    res: Response
    action: EventAction
  }): void {
    const context = HTTPSubscriber.getContext({ req: evt.req, res: evt.res })
    logger[evt.action.level](evt.action.message, {
      context,
      event: evt.action.body,
    })
  }

  public static onError(evt: {
    req: Request
    res: Response
    err: Error
  }): void {
    const context = HTTPSubscriber.getContext({ req: evt.req, res: evt.res })
    let backtrace: string[]
    if (evt.err.stack) {
      backtrace = evt.err.stack.split('\n')
    }
    logger.error(`(RuntimeError) ${evt.err.message}`, {
      context,
      event: {
        error: {
          name: 'RuntimeError',
          message: evt.err.message,
          backtrace,
        },
      },
    })
  }

  public static onClientWarning(evt: {
    req: Request
    res: Response
    err: Error
    details?: unknown
  }): void {
    const context = HTTPSubscriber.getContext({ req: evt.req, res: evt.res })
    logger.warn(`(ClientError) ${evt.err.message}`, {
      context,
      event: {
        client_error: {
          name: 'ClientError',
          message: evt.err.message,
          details: evt.details,
        },
      },
    })
  }

  private static genRequestID(): string {
    return crypto.randomBytes(16).toString('base64')
  }

  private static sanitizeHeaders(
    headers: Record<string, unknown>
  ): Record<string, unknown> {
    const ret = { ...headers }
    if (headers.cookie) {
      ret.cookie = '____'
    }
    if (headers['set-cookie']) {
      ret['set-cookie'] = '____'
    }
    return ret
  }

  public static getContext(evt: {
    req: Request
    res: Response
  }): RequestContext {
    const hostname =
      (evt.req.headers['x-forward-for'] as string) || evt.req.hostname
    const requestId = evt.res.locals.request_id || HTTPSubscriber.genRequestID()
    let user: RequestUserContext
    if (evt.req.session?.data) {
      const userInfo: UserSession = evt.req.session.data
      user = {
        lanid: userInfo.lanid,
        user: userInfo.firstName,
        role: userInfo.role,
        email: userInfo.email,
      }
    }

    return {
      http: {
        method: evt.req.method,
        path: evt.req.path,
        remote_host: hostname,
        request_id: requestId,
      },
      user,
    }
  }

  public static getHttpRequest(
    req: Request,
    headers: Record<string, unknown>
  ): HttpServerRequest {
    return {
      method: req.method,
      schema: req.secure ? 'https' : 'http',
      host: req.hostname,
      headers,
    }
  }

  public static getHttpResponse(
    res: Response,
    timeMs: number
  ): HttpServerResponse {
    return {
      request_id: res.locals.request_id,
      status: res.statusCode,
      time_ms: timeMs,
      headers: HTTPSubscriber.sanitizeHeaders(res.getHeaders()),
    }
  }
}

export default new HTTPSubscriber()
