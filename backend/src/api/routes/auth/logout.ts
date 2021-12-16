import { Request, Response, NextFunction } from 'express'
import { AsyncGet } from 'aejo'

export default AsyncGet({
  tags: ['auth'],
  description: 'Local User Logout',
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              logout: {
                type: 'boolean',
              },
            },
          },
        },
      },
    },
  },
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      req.session.destroy((err) => {
        if (err) {
          res.status(500).send({ message: 'failed' })
        }
        res.status(200).send({ logout: true })
        next()
      })
    },
  ],
})
