import { Request, Response } from 'express'
import httpEvent from '../../subscribers/http'
import {
  ValidationError,
  NotFoundError,
  DBError,
  ConstraintViolationError,
  UniqueViolationError,
  NotNullViolationError,
  ForeignKeyViolationError,
  CheckViolationError,
  DataError,
} from 'objection'

export default function (err: Error, req: Request, res: Response): boolean {
  const evt = { req, res, err }

  if (err instanceof ValidationError) {
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        reason: err.message,
        type: err.type,
        name: err.name,
      },
    })
    res.status(400).send({
      message: 'Validation error',
      type: 'ValidationError',
      data: {
        reason: err.message,
        path: err.data,
      },
    })
    return true
  }

  if (err instanceof NotFoundError) {
    httpEvent.emit('clientWarning', evt)
    res.status(404).send({
      message: 'Record not found',
      type: 'NotFound',
      data: {},
    })
    return true
  }

  if (err instanceof ConstraintViolationError) {
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        payload: req.body,
      },
    })
    res.status(400).send({
      message: 'A constraint violation error occured',
      type: 'ConstraintViolationError',
      data: err.message,
      name: err.name,
    })
    return true
  }

  if (err instanceof UniqueViolationError) {
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        payload: req.body,
      },
    })
    res.status(400).send({
      message: 'A unique constraint violation error occured',
      type: 'UniqueViolationError',
      data: err.message,
      name: err.name,
    })
    return true
  }

  if (err instanceof NotNullViolationError) {
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        payload: req.body,
      },
    })
    res.status(400).send({
      message: 'A not null violation error occured',
      type: 'NotNullViolationError',
      data: err.message,
      name: err.name,
    })
    return true
  }

  if (err instanceof ForeignKeyViolationError) {
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        payload: req.body,
      },
    })
    res.status(409).send({
      message: 'A foreign key error occured',
      type: 'ForeignKeyViolationError',
      data: {
        table: err.table,
        message: err.message,
      },
    })
    return true
  }

  if (err instanceof CheckViolationError) {
    httpEvent.emit('clientWarning', {
      ...evt,
      details: {
        payload: req.body,
      },
    })
    res.status(409).send({
      message: 'A check violation error occured',
      type: 'CheckViolationError',
      details: {
        table: err.table,
        message: err.message,
      },
    })
    return true
  }

  if (err instanceof DBError) {
    httpEvent.emit('error', evt)
    res.status(400).send({
      message: 'A database error occured. Please try again',
      type: 'DatabaseError',
      data: {},
    })
    return true
  }

  if (err instanceof DataError) {
    httpEvent.emit('error', {
      ...evt,
      details: {
        payload: req.body,
      },
    })
    res.status(400).send({
      message: 'A data error occured. Please try again.',
      type: 'DataError',
      data: {},
    })
    return true
  }

  return false
}
