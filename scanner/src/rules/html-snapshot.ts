import path from 'path'

import YaraSync from '../lib/yara-sync'
import * as MerryMaker from '@merrymaker/types'
import { Rule } from './base'

import LRUCache from 'lru-native2'
import crypto from 'crypto'


const twoWeeks = 1000 * 60 * 60 * 24 * 7 * 2
const yara = new YaraSync()

// eslint-disable-next-line @typescript-eslint/explicit-function-return-type
;(async () => {
  // init yara skimmer rules
  await yara.initAsync({
    rules: [{ filename: path.resolve(__dirname, 'skimmer.yara') }]
  })
})()

const snapshotHashCache = new LRUCache({
  maxElements: 10000,
  maxAge: twoWeeks,
  size: 50,
  maxLoadFactor: 2.0
})

/**
 * HTMLSnapShot rule
 *
 * Checks for malicious content in page content
 *
 */
export class HtmlSnapshotRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    const payload = scanEvent.payload as MerryMaker.HtmlSnapshot
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      message: 'no alert',
      level: this.options.level,
      context: { url: payload.url }
    }

    if (payload.html) {
      const hash = crypto.createHash('sha256').update(payload.html).digest('base64');
      if (snapshotHashCache.get(hash)) {
        res.message = `seen HTMLSnapshot (cache) ${hash}`
        return this.resolveEvent(res)
      }

      snapshotHashCache.set(hash, 1)

      const result = await yara.scanAsync({
        buffer: Buffer.from(payload.html, 'utf-8')
      })
      if (result.rules.length) {
        this.alertResults = result.rules.map(r => ({
          ...res,
          alert: true,
          message: `${r.id} hit`
        }))
      }
    }

    return this.resolveEvent(res)
  }
}

export default new HtmlSnapshotRule({
  name: 'HTMLSnapshotRule',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects malicious page content'
})
