import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'
import AuthService from '../../../services/auth'

export default AsyncGet({
  tags: ['auth'],
  description: 'Determine if session is active',
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              lanid: {
                type: 'string',
                description: 'User Login/LanID',
              },
              role: {
                type: 'string',
                description: 'User role',
                enum: ['user', 'admin', 'transport'],
              },
              firstName: {
                type: 'string',
                description: 'User First Name',
              },
              lastName: {
                type: 'string',
                description: 'User Last Name',
              },
              email: {
                type: 'string',
                description: 'User Email',
              },
              isAuth: {
                type: 'boolean',
                description: 'Session Auth flag',
              },
              exp: {
                type: 'integer',
                description: 'Unix epoch when session expires',
              },
            },
          },
        },
      },
    },
    '401': {
      description: 'Not authenticated',
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      if (AuthService.isAuth(req.session)) {
        res.status(200).send(req.session.data)
      } else {
        res.status(401).send({ message: 'not_authenticated' })
      }
      next()
    },
  ],
})
