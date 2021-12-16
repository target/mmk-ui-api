import * as tls from 'tls'
import fs from 'fs'
import { Kafka } from 'kafkajs'
import { config } from 'node-config-ts'

import logger from '../loaders/logger'

import { AlertSinkBase, AlertEvent } from './base'

export interface AlertV1 {
  rule: string
  level: AlertEvent['type']
  description: string
  cause?: string
  url?: string
  scanName?: string
  scanUrl: string
  siteId?: string
}

let kafkaSSLOptions: tls.ConnectionOptions

const toAlertV1 = (evt: AlertEvent): AlertV1 => ({
  rule: evt.name,
  level: evt.type,
  description: evt.message,
  scanUrl: `${config.server.uri}/scans/${evt.scan_id}`,
})

/**
 * init
 *
 * initialize the kafka client producer
 * and return a method for sending a RuleAlertEvent
 */
export const init = (
  kafkaConfig: typeof config.alerts.kafka
): ((evt: AlertEvent) => Promise<boolean>) => {
  if (!kafkaConfig.enabled) return

  try {
    kafkaSSLOptions = {
      key: [fs.readFileSync(config.alerts.kafka.key)],
      cert: [fs.readFileSync(config.alerts.kafka.cert)],
      ca: [fs.readFileSync(config.server.ca)],
      rejectUnauthorized: false,
      secureProtocol: 'TLSv1_2_method',
    }
  } catch (e) {
    logger.error({
      task: 'kafka-alerter/init',
      error: e.message,
    })
  }

  const kafka = new Kafka({
    brokers: [`${kafkaConfig.host}:${kafkaConfig.port}`],
    clientId: kafkaConfig.clientID,
    ssl: kafkaSSLOptions,
  })

  const kafkaProducer = kafka.producer()

  return async (evt: AlertEvent): Promise<boolean> => {
    await kafkaProducer.connect()
    const messages = [
      {
        key: 'msg',
        value: JSON.stringify(toAlertV1(evt)),
      },
    ]
    await kafkaProducer.send({
      topic: kafkaConfig.topic,
      messages,
    })
    await kafkaProducer.disconnect()
    logger.info({
      task: 'kafka-alert/send',
      result: 'sent',
    })
    return true
  }
}

export default {
  name: 'Kafka Alert Sink',
  enabled: config.alerts?.kafka?.enabled === true,
  send: init(config.alerts?.kafka),
} as AlertSinkBase
