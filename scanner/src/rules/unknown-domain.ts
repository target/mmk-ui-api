// Unknown domain rule
import { parse } from 'tldts'
import LRUCache from 'lru-native2'
import * as MerryMaker from '@merrymaker/types'
import { Rule } from './base'
import { IResult } from 'tldts-core'

const oneHour = 1000 * 60 * 60

const domainAllowListCache = new LRUCache({
  maxElements: 1000,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0,
})

const seenDomainCache = new LRUCache({
  maxElements: 10000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0,
})

export class UnknownDomainRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  async process(
    payload: MerryMaker.WebRequestEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      level: this.options.level,
      context: { url: payload.url },
    }

    // Handle invalid URLs
    let payloadURL: IResult
    try {
      payloadURL = parse(payload.url)
    } catch (e) {
      res.message = `failed to parse URL ${e.message}`
      return this.resolveEvent(res)
    }

    if (payloadURL.domain.length === 0) {
      res.message = `missing / empty domain for payload (${payload.url})`
      return this.resolveEvent(res)
    }

    // Check allow_list cache
    if (domainAllowListCache.get(payloadURL.domain)) {
      res.message = `allow-listed (cache) ${payloadURL.domain}`
      return this.resolveEvent(res)
    }

    const whiteList = await this.fetchRemoteAllowList(payloadURL.domain, 'fqdn')

    // Checkout backend transport
    if (whiteList.total > 0) {
      res.message = `allow-listed (DB) ${payloadURL.domain}`
      domainAllowListCache.set(payloadURL.domain, 1)
      return this.resolveEvent(res)
    }
    // Check full hostname in seen domain cache
    if (seenDomainCache.get(payloadURL.hostname) === 1) {
      res.message = `seen hostname (cache) ${payloadURL.hostname}`
      return this.resolveEvent(res)
    }

    // Check remote cache
    const seenData = await this.bumpRemoteCache(payloadURL.hostname, 'domain')

    // cache the result
    seenDomainCache.set(payloadURL.hostname, 1)
    // Alert if domain was not found in any store
    if (seenData.store === 'none') {
      res.alert = true
      res.message = `${payloadURL.hostname} unknown`
    }

    return this.resolveEvent(res)
  }
}

export default new UnknownDomainRule({
  name: 'unknown.domain',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects domains that have not been seen',
})
