import path from 'path'
import https from 'https'
import url, { URLSearchParams } from 'url'
import LRUCache from 'lru-native2'
import { config } from 'node-config-ts'
import { js } from 'js-beautify'
import YaraSync from '../lib/yara-sync'
import * as MerryMaker from '@merrymaker/types'
import fetch, { RequestInit } from 'node-fetch'
import { Rule, storeNameResponseSchema, StoreTypeResponse } from './base'
import logger from '../loaders/logger'
import { isOfType } from '../lib/utils'

const oneHour = 1000 * 60 * 60

const yara = new YaraSync()

const httpsAgent = new https.Agent({
  rejectUnauthorized: false,
})

// eslint-disable-next-line @typescript-eslint/explicit-function-return-type
;(async () => {
  // init yara skimmer rules
  await yara.initAsync({
    rules: [{ filename: path.resolve(__dirname, 'skimmer.yara') }],
  })
})()

const seenFilesCache = new LRUCache({
  maxElements: 10000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0,
})

export class YaraRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    const payload = scanEvent.payload as MerryMaker.WebScriptEvent
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      level: this.options.level,
      context: { url: payload.url },
    }

    if (seenFilesCache.get(payload.sha256)) {
      res.message = `seen file hash (cache) ${payload.sha256}`
      return this.resolveEvent(res)
    }

    const remoteCache = await this.fetchRemoteSeenStrings(payload.sha256)

    if (remoteCache.store !== 'none') {
      res.message = `seen file (remoteCache.store)`
      return this.resolveEvent(res)
    }

    const fileBuff = await this.fetchFile(payload)
    logger.debug({
      rule: 'yara',
      message: `fetched buff length (${fileBuff.length})`,
    })

    const source: string = js(fileBuff)
    if (source.length > 0) {
      // need to convert to a string
      logger.debug({
        rule: 'yara',
        message: `yara js buffer len (${source.length})`,
      })
      const result = await yara.scanAsync({
        buffer: Buffer.from(source, 'utf-8'),
      })

      if (result.rules.length) {
        this.alertResults = result.rules.map((r) => ({
          ...res,
          alert: true,
          message: `${r.id} hit`,
        }))
      }
    } else {
      logger.warn({
        rule: 'yara',
        message: `warning: ${payload.url} - return 0 bytes "${source}"`,
      })
    }
    // Update redis
    seenFilesCache.set(payload.sha256, 1)
    // update remote cache
    await this.bumpRemoteCache(payload.sha256, 'hash')
    if (this.alertResults.length) {
      return this.alertResults
    }
    return this.resolveEvent(res)
  }

  async fetchRemoteSeenStrings(hash: string): Promise<StoreTypeResponse> {
    const params = new URLSearchParams({
      key: hash,
      field: 'key',
      type: 'hash',
    })
    const seenHash = await fetch(
      `${config.transport.http}/api/seen_strings/_cache?${params}`,
      {
        method: 'get',
      }
    )
    const res = await seenHash.json()
    if (isOfType<StoreTypeResponse>(res, storeNameResponseSchema)) {
      return res
    }
  }

  async fetchFile(payload: MerryMaker.WebScriptEvent): Promise<string> {
    const opts: RequestInit = {
      method: 'get',
      headers: payload.headers,
    }
    const scriptURL = url.parse(payload.url)

    if (scriptURL.protocol === 'https:') {
      opts.agent = httpsAgent
    }
    logger.info({
      rule: 'yara',
      message: `fetchFile URL ${payload.url}`,
    })
    const res = await fetch(payload.url, opts)
    logger.debug({
      rule: 'yara',
      message: 'fetchFile Response',
      context: {
        statusText: res.statusText,
        status: res.status,
      },
    })

    return res.text()
  }
}

export default new YaraRule({
  name: 'yara',
  level: 'prod',
  alert: false,
  context: {},
  description: 'static analysis of known malicious JS',
})
