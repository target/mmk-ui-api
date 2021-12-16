import { Request, Response } from 'express'
import { ClientError } from './client-errors'
import httpEvent from '../../subscribers/http'

export default function (err: unknown, req: Request, res: Response): boolean {
  if (!(err instanceof ClientError)) {
    return false
  }
  switch (err.context.type) {
    case 'forbidden':
      res
        .status(403)
        .send({ message: err.message, type: 'Forbidden', data: err.context })
      break
    case 'unauthorized':
    case 'invalid_creds':
      res
        .status(401)
        .send({ message: err.message, type: 'Unauthorized', data: err.context })
      break
    default:
      res
        .status(422)
        .send({ message: err.message, type: 'ClientError', data: err.context })
  }
  httpEvent.emit('clientWarning', {
    req,
    res,
    err,
    details: err.context,
  })
  return true
}
