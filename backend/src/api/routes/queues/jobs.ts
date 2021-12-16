import { NextFunction, Request, Response } from 'express'
import { AsyncGet } from 'aejo'
import { redisClient } from '../../../repos/redis'

export default AsyncGet({
  tags: ['queues'],
  description: 'Fetch queue counts',
  responses: {
    '200': {
      description: 'Ok',
      content: {
        'application/json': {
          schema: {
            type: 'object',
            properties: {
              event: {
                description: 'Number of events waiting to be processed',
                type: 'integer',
              },
              scanner: {
                description: 'Number of active scans',
                type: 'integer',
              },
              schedule: {
                description: 'Number of scheduled scans waiting to be run',
                type: 'integer',
              },
            },
          },
        },
      },
    },
    '404': {
      description: 'Queue missing from cache',
    },
  },
  middleware: [
    async (_req: Request, res: Response, next: NextFunction): Promise<void> => {
      const queues = await redisClient.get('job-queue')
      if (queues !== null) {
        res.status(200).send(JSON.parse(queues))
      } else {
        res.sendStatus(404)
      }
      next()
    },
  ],
})
