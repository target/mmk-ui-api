import { default as Pino } from 'pino'
import { config } from 'node-config-ts'

export default Pino({
  name: 'mmk-scanner',
  level: config.env === 'test' ? 'silent' : 'debug',
})
