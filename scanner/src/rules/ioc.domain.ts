import { parse } from 'tldts'
import LRUCache from 'lru-native2'

import * as MerryMaker from '@merrymaker/types'
import { Rule } from './base'

const oneHour = 1000 * 60 * 60

const iocDomainCache = new LRUCache({
  maxElements: 1000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0,
})

const domainAllowListCache = new LRUCache({
  maxElements: 1000,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0,
})

/**
 * IOCDomainRule
 *
 * Checks for known bad domains (IOCS)
 *
 * 1. Check local and remote allow-listed cache, don't alert if found
 * 2. Check local and and remote IOC cache (domain), alert if found and update cache
 */
export class IOCDomainRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
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
      context: { url: payload.url },
    }
    // Ignore images
    if (payload.resourceType === 'image') {
      res.message = 'resourceType = image'
      return this.resolveEvent(res)
    }
    // check local allow-listed cache
    const payloadURL = parse(payload.url)
    if (domainAllowListCache.get(payloadURL.domain)) {
      res.message = `allow-listed (cache) ${payloadURL.domain}`
      return this.resolveEvent(res)
    }

    // check remote allow-listed cache
    const allowListed = await this.fetchRemoteAllowList(
      payloadURL.domain,
      'fqdn'
    )

    if (allowListed.total > 0) {
      res.message = `allow-listed (DB) ${payloadURL.domain}`
      domainAllowListCache.set(payloadURL.domain, 1)
      return this.resolveEvent(res)
    }

    if (iocDomainCache.get(payloadURL.hostname)) {
      res.alert = true
      res.message = `known IOC (cache) ${payloadURL.hostname}`
      return this.resolveEvent(res)
    }

    const iocs = await this.fetchRemoteIOC(payloadURL.hostname)
    if (iocs.total > 0) {
      res.alert = true
      res.message = `known IOC (DB) ${payloadURL.hostname}`
      iocDomainCache.set(payloadURL.hostname, 1)
    }
    return this.resolveEvent(res)
  }
}

export default new IOCDomainRule({
  name: 'ioc.domain',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects known malicious domains',
})
