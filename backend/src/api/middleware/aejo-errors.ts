import { Request, Response } from 'express'
import { ValidationError } from 'aejo/dist/lib/errors'

import httpEvent from '../../subscribers/http'

export default function (err: Error, req: Request, res: Response): boolean {
  const evt = { req, res, err }
  if (err instanceof ValidationError) {
    const context = err.context[0]
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        reason: err.message,
        type: context.keyword,
        name: context.message,
      },
    })
    res.status(422).send({
      message: 'Validation error',
      type: 'ValidationError',
      data: {
        reason: `${context.instancePath.substr(1)}: ${context.message}`,
        path: context.instancePath,
      },
    })
    return true
  }
}
