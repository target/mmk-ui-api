// Google Analytics rule
import { parse } from 'tldts'
import { URL } from 'url'
import LRUCache from 'lru-native2'
import * as MerryMaker from '@merrymaker/types'
import { Rule } from './base'

const oneHour = 1000 * 60 * 60
const GOOGLE_ANALYTICS = 'google-analytics.com'

// we are looking for GoogleTID
// trackingID

const trackingIDAllowListCache = new LRUCache({
  maxElements: 1000,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0,
})

const seenTrackingIDCache = new LRUCache({
  maxElements: 10000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0,
})

export class GoogleAnalyticsRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    const payload = scanEvent.payload as MerryMaker.WebRequestEvent
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      level: this.options.level,
      context: { url: payload.url },
    }

    const payloadURL = parse(payload.url)
    let tid = null
    if (payloadURL.domain === GOOGLE_ANALYTICS) {
      if (payload.method === 'GET') {
        const url = new URL(payload.url)
        if (url.searchParams.get('tid')) {
          tid = url.searchParams.get('tid')
        }
      } else if (payload.method === 'POST') {
        // hacky, but it works
        const url = new URL(`${payload.url}/?${payload.postData}`)
        if (url.searchParams.get('tid')) {
          tid = url.searchParams.get('tid')
        }
      }

      if (tid) {
        // Check allow_list cache
        if (trackingIDAllowListCache.get(tid)) {
          res.message = `allow-listed (cache) ${tid}`
          return this.resolveEvent(res)
        }

        const allowList = await this.fetchRemoteAllowList(
          tid,
          'google-analytics'
        )
        // Checkout backend transport
        if (allowList.total > 0) {
          res.message = `allow-listed (DB) ${tid}`
          trackingIDAllowListCache.set(tid, 1)
          return this.resolveEvent(res)
        } else {
          // post to remoteAllowList
          await this.addRemoteAllowList(tid, 'google-analytics')
        }

        // Check TID in seen domain cache
        if (seenTrackingIDCache.get(tid) === 1) {
          res.message = `seen TID (cache) ${tid}`
          return this.resolveEvent(res)
        }
        // Check remote cache
        const seenData = await this.bumpRemoteCache(tid, 'google-analytics')
        // cache the result
        seenTrackingIDCache.set(tid, 1)

        // Alert if domain was not found in any store
        if (seenData.store === 'none') {
          res.alert = true
          res.message = `New Google Analytics: ${tid}`
        }
      }
    }
    return this.resolveEvent(res)
  }
}

export default new GoogleAnalyticsRule({
  name: 'google.analytics',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects new Google Analytics accounts',
})
