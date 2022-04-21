// Unknown domain rule
import { parse } from 'tldts'
import LRUCache from 'lru-native2'
import * as MerryMaker from '@merrymaker/types'
import { Rule } from './base'
import { IResult } from 'tldts-core'

const oneHour = 1000 * 60 * 60

export const domainAllowListCache = new LRUCache<number>({
  maxElements: 1000,
  maxAge: oneHour,
  size: 50,
  maxLoadFactor: 2.0
})

export const seenDomainCache = new LRUCache<number>({
  maxElements: 10000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0
})

export class UnknownDomainRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  payload: MerryMaker.WebRequestEvent
  payloadURL: IResult
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    this.event = scanEvent

    this.payload = scanEvent.payload as MerryMaker.WebRequestEvent
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      level: this.options.level,
      context: { url: this.payload.url }
    }

    // Handle invalid URLs
    try {
      this.payloadURL = parse(this.payload.url)
    } catch (e) {
      res.message = `failed to parse URL ${e.message}`
      return this.resolveEvent(res)
    }

    if (this.payloadURL.domain === null) {
      res.message = `missing / empty domain for payload (${this.payload.url})`
      return this.resolveEvent(res)
    }

    // Check allow_list cache
    const allowedDomain = await this.isAllowed({
      value: this.payloadURL.domain,
      key: 'fqdn',
      cache: domainAllowListCache
    })

    if (allowedDomain) {
      res.message = `domain allow-listed ${this.payloadURL.domain}`
      return this.resolveEvent(res)
    }

    // Check if referer allows pass through
    if (this.payload.headers?.referer) {
      const { allowed, message } = await this.allowedReferrer()
      if (allowed) {
        res.message = message
        return this.resolveEvent(res)
      }
    }

    const seenDomain = await this.wasSeen({
      value: this.payloadURL.hostname,
      key: 'domain',
      cache: seenDomainCache
    })

    // Alert if domain was not found in any store
    if (seenDomain.store === 'none') {
      res.alert = true
      res.message = `${this.payloadURL.hostname} unknown`
    }

    // attach domain
    res.context.domain = this.payloadURL.hostname

    return this.resolveEvent(res)
  }

  /**
   * allowedReferrer
   *
   * checks if request comes from an allowed referrer
   *
   * checks local `domainAllowListCache` then remote allow-list if not found
   *
   * updates `domainAllowListCache` if found in remote allow-list
   *
   * scopes test scans by `scanID`
   */
  async allowedReferrer(): Promise<{ allowed: boolean; message?: string }> {
    const refererURL = parse(this.payload.headers.referer)
    let lruKey = `${this.payloadURL.domain}|${refererURL.domain}`
    // check local cache before checking remote allow-list
    if (domainAllowListCache.get(lruKey)) {
      return {
        allowed: true,
        message: `allow-listed / referer (cache) ${lruKey}`
      }
    }
    const allowedReferrer = await this.fetchRemoteAllowList(
      refererURL.domain,
      'referrer'
    )
    if (allowedReferrer.total > 0) {
      // scope test to this scan
      if (this.event.test) {
        lruKey = `${lruKey}|${this.event.scanID}`
      }
      domainAllowListCache.set(lruKey, 1)
      return {
        allowed: true,
        message: `allow-listed / referer (${refererURL.domain}) (DB)`
      }
    }
    return { allowed: false }
  }
}

export default new UnknownDomainRule({
  name: 'unknown.domain',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects domains that have not been seen'
})
