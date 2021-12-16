import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import { User } from '../../../models'
import { QueryParam } from 'aejo'
import {
  listHandler,
  listResponseSchema,
  ListQueryParams,
} from '../../crud/list'
import { QueryBuilder } from 'objection'
import { userSchema } from './schemas'

const selectable = User.selectAble()

const List = [
  QueryParam({
    name: 'role',
    description: 'filter on user role',
    schema: {
      type: 'string',
      enum: ['user', 'transport', 'admin'],
    },
  }),
  QueryParam({
    name: 'login',
    description: 'filter on login using wildcard matching',
    schema: {
      type: 'string',
      minLength: 1,
    },
  }),
  ...ListQueryParams,
  QueryParam({
    name: 'fields',
    description: 'Select fields from results',
    schema: {
      type: 'array',
      items: {
        type: 'string',
        enum: selectable,
      },
    },
  }),
]

export default AsyncGet({
  tags: ['users'],
  description: 'List Users',
  parameters: List,
  responses: {
    '200': {
      description: 'OK',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: listResponseSchema(userSchema),
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      res.locals.whereBuilder = (builder: QueryBuilder<User>) => {
        if (req.query.role && typeof req.query.role === 'string') {
          builder.where('role', req.query.role)
        }
        if (req.query.login && typeof req.query.login === 'string') {
          builder.whereRaw('login iLIKE ?', [req.query.login.replace('*', '%')])
        }
      }
      next()
    },
    listHandler<User>(User, selectable),
  ],
})
