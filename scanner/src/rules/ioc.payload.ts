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

const payloadAllowListCache = new LRUCache({
  maxElements: 1000,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0,
})

const iocPayloadCache = new LRUCache({
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
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
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

    let payloadURL: IResult
    try {
      payloadURL = parse(payload.url)
    } catch (e) {
      res.message = 'unable to parse url'
      return this.resolveEvent(res)
    }

    if (payloadURL.hostname === null) {
      logger.warn({
        rule: 'ioc.payload',
        message: `unable to parse request ${payload}`,
      })
      res.message = 'unable to parse / not a URL'
      return this.resolveEvent(res)
    }

    // pass through allowed ioc payload domains
    if (payloadAllowListCache.get(payloadURL.hostname)) {
      res.message = `allow-listed (cache) ${payloadURL.domain}`
      return this.resolveEvent(res)
    }

    logger.info({
      rule: 'ioc.payload',
      message: 'fetching from remote allow list',
    })
    // check remote
    const allowListed = await this.fetchRemoteAllowList(
      payloadURL.hostname,
      'ioc-payload-domain'
    )

    if (allowListed.total > 0) {
      res.message = `allow-listed (DB) ${payloadURL.hostname}`
      payloadAllowListCache.set(payloadURL.hostname, 1)
      return this.resolveEvent(res)
    }

    const postData = payload.postData === undefined ? '' : payload.postData

    // combine url, headers, and post data and more things
    const combined = `postData:${postData} ${JSON.stringify(payload)}`
    const combinedHash = crypto
      .createHash('sha256')
      .update(combined)
      .digest('hex')

    if (iocPayloadCache.get(combinedHash)) {
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

    iocPayloadCache.set(combinedHash, 1)
    // do not update remote cache (these values have extremely high entropy)

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
