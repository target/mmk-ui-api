import { AsyncGet, QueryParam } from 'aejo'
import { Request, Response, NextFunction } from 'express'
import sub from 'date-fns/sub'
import AlertService from '../../../services/alert'

export default AsyncGet({
  tags: ['alerts'],
  description: 'Alert Aggregation',
  parameters: [
    QueryParam({
      name: 'interval_hours',
      description: 'Group alerts by hours',
      schema: {
        type: 'number',
        minimum: 1,
        default: 1
      }
    }),
    QueryParam({
      name: 'start_time',
      description: 'Start range',
      schema: {
        type: 'string',
        format: 'date-time',
        default: 'one week ago'
      }
    }),
    QueryParam({
      name: 'end_time',
      description: 'Start range',
      schema: {
        type: 'string',
        format: 'date-time',
        default: 'now'
      }
    })
  ],
  middleware: [
    async (req: Request, res: Response, next: NextFunction): Promise<void> => {
      const { interval_hours, start_time, end_time } = req.query
      let local_interval = 1
      if (interval_hours && typeof interval_hours == 'number') {
        local_interval = interval_hours
      }
      let local_start_time = sub(new Date(), { weeks: 1 })
      if (start_time) {
        local_start_time = new Date(local_start_time)
      }
      let local_end_time = new Date()
      if (end_time && typeof end_time == 'string') {
        local_end_time = new Date(end_time)
      }
      const agg = await AlertService.dateHist({
        starttime: local_start_time,
        endtime: local_end_time,
        interval: local_interval
      })
      res.set('Cache-Control', 'no-store')
      res.status(200).send({ rows: agg.rows })
      next()
    }
  ]
})
