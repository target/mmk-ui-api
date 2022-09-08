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

export const iocDomainCache = new LRUCache<number>({
  maxElements: 1000,
  maxAge: oneHour,
  size: 1000,
  maxLoadFactor: 2.0
})

export class WebsocketRule extends Rule {
  alertResults: MerryMaker.RuleAlert[]
  payloadURL: IResult
  async process(
    scanEvent: MerryMaker.ScanEvent
  ): Promise<MerryMaker.RuleAlert[]> {
    this.event = scanEvent
    const payload = scanEvent.payload as MerryMaker.WebFunctionCallEvent
    this.alertResults = []
    const res: MerryMaker.RuleAlert = {
      name: this.options.name,
      alert: false,
      level: this.options.level,
      context: { url: payload.args }
    }

    if (payload.func.toLocaleLowerCase() !== 'websocket') {
      res.message = `received non-websocket function call ${payload.func}`
      return this.resolveEvent(res)
    }

    const url = payload.args.toString()
    // Handle invalid URLs
    try {
      this.payloadURL = parse(url)
    } catch (e) {
      res.message = `failed to parse URL ${e.message}`
      return this.resolveEvent(res)
    }

    if (
      typeof this.payloadURL.domain !== 'string' ||
      this.payloadURL.domain.length === 0
    ) {
      res.message = `missing / empty domain for payload (${url})`
      return this.resolveEvent(res)
    }

    // Check allow_list cache
    const allowedDomain = await this.isAllowed({
      value: this.payloadURL.domain,
      key: 'fqdn',
      cache: domainAllowListCache
    })

    if (allowedDomain) {
      res.message = `allow-listed (cache) ${this.payloadURL.domain}`
      return this.resolveEvent(res)
    }

    // Check if it is an IOC first
    if (iocDomainCache.get(this.payloadURL.hostname)) {
      res.alert = true
      res.message = `known IOC (cache) ${this.payloadURL.hostname}`
      return this.resolveEvent(res)
    }

    const iocs = await this.fetchRemoteIOC(this.payloadURL.hostname)
    if (iocs.total > 0) {
      res.alert = true
      res.message = `known IOC (DB) ${this.payloadURL.hostname}`
      iocDomainCache.set(this.scopeValue(this.payloadURL.hostname), 1)
      return this.resolveEvent(res)
    }

    const seenDomain = await this.wasSeen({
      value: this.payloadURL.hostname,
      key: 'domain',
      cache: seenDomainCache
    })

    // Check full hostname in seen domain cache
    if (seenDomain.store !== 'none') {
      res.message = `seen hostname (cache) ${this.payloadURL.hostname}`
      return this.resolveEvent(res)
    }

    res.alert = true
    res.message = `${this.payloadURL.hostname} unknown`

    return this.resolveEvent(res)
  }
}

export default new WebsocketRule({
  name: 'domain.via.websocket',
  level: 'prod',
  alert: false,
  context: {},
  description: 'detects unknown/IOC domains in websocket connections'
})
