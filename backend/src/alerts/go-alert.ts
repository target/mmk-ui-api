/* HTTP alert type */
import fetch from 'node-fetch'
import fs from 'fs'
import https from 'https'
import queryString from 'querystring'

import { config } from 'node-config-ts'
import logger from '../loaders/logger'
import { AlertSinkBase, AlertEvent } from './base'

const MAX_GO_ALERT_LEN = 128

/**
 * queryFromAlert
 *
 * formats AlertEvent into GoAlert query string
 */
export const queryFromAlert = (evt: AlertEvent, token: string): string =>
  queryString.stringify({
    summary: `${evt.name} - ${evt.message}`.substring(0, MAX_GO_ALERT_LEN),
    details: `${evt.details}`.substring(0, MAX_GO_ALERT_LEN),
    token,
  })

/**
 * goAlert
 *
 * sends goAlert message from AlertEvent
 */
export const init = (goAlertConfig: typeof config.alerts.goAlert) => async (
  evt: AlertEvent
): Promise<boolean> => {
  logger.info({
    task: 'go-alert/send',
    action: 'requested to send alert',
  })
  if (!goAlertConfig.enabled) return

  const agent = new https.Agent({
    rejectUnauthorized: true,
    ca: [fs.readFileSync(config.server.ca)],
  })

  const query = queryFromAlert(evt, goAlertConfig.token)
  try {
    const res = await fetch(`${goAlertConfig.url}?${query}`, {
      method: 'post',
      agent,
    })
    const body = await res.text()
    logger.info({
      task: 'go-alert/send',
      result: body,
    })
    return true
  } catch (e) {
    logger.error({
      task: 'go-alert/send',
      error: e.message,
    })
    throw e
  }
}

export default {
  name: 'HTTP Alert Sink',
  enabled: config.alerts.goAlert?.enabled === true,
  send: init(config.alerts.goAlert),
} as AlertSinkBase
