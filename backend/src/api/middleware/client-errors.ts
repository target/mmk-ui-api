type ErrorContextTypes =
  | 'timeout'
  | 'client'
  | 'forbidden'
  | 'unauthorized'
  | 'invalid_creds'

interface ClientErrorContext {
  type: ErrorContextTypes
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  event: any
}

export class ClientError extends Error {
  public readonly context: ClientErrorContext
  constructor(message: string, context?: ClientErrorContext) {
    super(message)
    this.context = context || { type: 'client', event: 'general' }
    Object.setPrototypeOf(this, ClientError.prototype)
    Error.captureStackTrace(this, ClientError)
  }
}

export class ForbiddenError extends ClientError {
  constructor(resource: string, access: unknown) {
    super('You do not have permission to perform this action', {
      type: 'forbidden',
      event: { resource, access },
    })
  }
}

export class InvalidCreds extends ClientError {
  constructor(resource: string, access: unknown) {
    super('Your login/password was not accepted', {
      type: 'invalid_creds',
      event: { resource, access },
    })
  }
}

export class UnauthorizedError extends ClientError {
  constructor(resource: string, user: string) {
    super(`You must be logged in to access this resouce - ${resource}`, {
      type: 'unauthorized',
      event: { user },
    })
  }
}
