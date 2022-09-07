// IOC Payload rule
import { parse } from 'tldts'
import crypto from 'crypto'
import path from 'path'
import { IResult } from 'tldts-core'
import LRUCache from 'lru-native2'
import MerryMaker from '@merrymaker/types'
import { Rule } from './base'
import YaraSync from '../lib/yara-sync'
import logger from '../loaders/logger'

const yara = new YaraSync()
const oneHour = 1000 * 60 * 60

export const payloadAllowListCache = new LRUCache<number>({
  maxElements: 1000,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0,
})

export const iocPayloadCache = new LRUCache<number>({
  maxElements: 1000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0,
})

// eslint-disable-next-line @typescript-eslint/explicit-function-return-type
;(async () => {
  // init yara skimmer rules
  await yara.initAsync({
    rules: [{ filename: path.resolve(__dirname, 'ioc.payloads.yara') }],
  })
})()

/**
 * IOCPayloadRule
 *
 * Checks for known IOCs in requests
 */
export class IOCPayloadRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  payloadURL: IResult
  combinedHash: string
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    this.event = scanEvent
    const payload = scanEvent.payload as MerryMaker.WebRequestEvent
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      message: 'no alert',
      level: this.options.level,
      context: {
        url: payload.url,
        body: payload.postData,
        header: payload.headers,
      },
    }

    try {
      this.payloadURL = parse(payload.url)
    } catch (e) {
      res.message = 'unable to parse url'
      return this.resolveEvent(res)
    }

    if (this.payloadURL.hostname === null) {
      logger.warn({
        rule: 'ioc.payload',
        message: `unable to parse request ${payload}`,
      })
      res.message = 'unable to parse / not a URL'
      return this.resolveEvent(res)
    }

    // Check allow_list cache
    const allowedHostname = await this.isAllowed({
      value: this.payloadURL.hostname,
      key: 'ioc-payload-domain',
      cache: payloadAllowListCache
    })

    if (allowedHostname) {
      res.message = `allow-listed (cache) ${this.payloadURL.domain}`
      return this.resolveEvent(res)
    }

    const postData = payload.postData === undefined ? '' : payload.postData

    // combine url, headers, and post data and more things
    const combined = `postData:${postData} ${JSON.stringify(payload)}`

    this.combinedHash = crypto
      .createHash('sha256')
      .update(combined)
      .digest('hex')

    if (this.seenLocal({ value: this.combinedHash, cache: iocPayloadCache })) {
      res.message = `seen request payload (cache)`
      return this.resolveEvent(res)
    }

    const result = await yara.scanAsync({
      buffer: Buffer.from(combined, 'utf-8'),
    })

    if (result.rules.length) {
      this.alertResults = result.rules.map((r) => ({
        ...res,
        alert: true,
        message: `${r.id} hit`,
      }))
    }

    iocPayloadCache.set(this.scopeValue(this.combinedHash), 1)

    if (this.alertResults.length) {
      return this.alertResults
    }

    return this.resolveEvent(res)
  }
}

export default new IOCPayloadRule({
  name: 'ioc.payload',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects known sensitive payment data',
})
