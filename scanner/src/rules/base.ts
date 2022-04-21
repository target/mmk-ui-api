import fetch from 'node-fetch'
import MerryMakerTypes, { ScanEvent } from '@merrymaker/types'
import LRUCache from 'lru-native2'
import { config } from 'node-config-ts'

import { isOfType } from '../lib/utils'
import logger from '../loaders/logger'

const allowListURL = `${config.transport.http}/api/allow_list`

export const totalResponseSchema = {
  type: 'object',
  properties: {
    total: { type: 'number' }
  },
  required: ['total']
}

export const storeTotalResponseSchema = {
  type: 'object',
  properties: {
    store: { type: 'number' }
  },
  required: ['store']
}

export const storeNameResponseSchema = {
  type: 'object',
  properties: {
    store: { type: 'string', enum: ['local', 'redis', 'database', 'none'] }
  },
  required: ['store']
}

export type StoreTypeResponse = {
  store: 'local' | 'redis' | 'database' | 'none'
}

export abstract class Rule {
  event: ScanEvent
  alertResults: MerryMakerTypes.RuleAlert[]
  constructor(protected readonly options: MerryMakerTypes.RuleAlert) {}
  abstract process(scanEvent: ScanEvent): Promise<MerryMakerTypes.RuleAlert[]>
  get ruleDetails(): MerryMakerTypes.RuleAlert {
    return this.options
  }
  async resolveEvent(
    evt: MerryMakerTypes.RuleAlert
  ): Promise<MerryMakerTypes.RuleAlert[]> {
    this.alertResults.push(evt)
    return Promise.resolve(this.alertResults)
  }
  async fetchRemoteAllowList(
    key: string,
    type: string
  ): Promise<{ total: number }> {
    const allow = await fetch(
      `${allowListURL}/?key=${key}&type=${type}&field=key`
    )
    const res = await allow.json()
    if (isOfType<{ total: number }>(res, totalResponseSchema)) {
      return res
    }
  }

  async fetchRemoteIOC(hostname: string): Promise<{ total: number }> {
    const remoteIOC = await fetch(
      `${config.transport.http}/api/iocs/?type=fqdn&value=${hostname}`
    )
    const res = await remoteIOC.json()
    if (isOfType<{ total: number }>(res, totalResponseSchema)) {
      return res
    }
  }

  async addRemoteAllowList(
    key: string,
    type: string
  ): Promise<{ store: number }> {
    const addReq = await fetch(allowListURL, {
      method: 'post',
      body: JSON.stringify({
        allow_list: {
          key,
          type
        }
      }),
      headers: { 'Content-Type': 'application/json' }
    })
    const res = await addReq.json()
    if (isOfType<{ store: number }>(res, storeTotalResponseSchema)) {
      return res
    }
  }

  async bumpRemoteCache(key: string, type: string): Promise<StoreTypeResponse> {
    // bump remote cache
    const seenReq = await fetch(
      `${config.transport.http}/api/seen_strings/_cache`,
      {
        method: 'post',
        body: JSON.stringify({
          seen_string: {
            key,
            type
          }
        }),
        headers: { 'Content-Type': 'application/json' }
      }
    )
    const res = await seenReq.json()
    if (isOfType<StoreTypeResponse>(res, storeNameResponseSchema)) {
      return res
    }
  }

  /**
   * fetchSeenStrings
   *
   * read-only call to check if seen_string exists in remote cache
   */
  async fetchSeenStrings(
    key: string,
    type: string
  ): Promise<StoreTypeResponse> {
    const url = new URL(`${config.transport.http}/api/seen_strings/_cache`)
    url.search = new URLSearchParams({
      key,
      type
    }).toString()
    // fetch cache
    const seenReq = await fetch(url, {
      method: 'get',
      headers: { 'Content-Type': 'application/json' }
    })
    const res = await seenReq.json()
    if (isOfType<StoreTypeResponse>(res, storeNameResponseSchema)) {
      return res
    }
  }

  /**
   * isAllowed
   *
   * check to see if key/value is found in remote allow list
   *
   * updates `cache` if found
   */
  async isAllowed(options: {
    value: string
    key: string
    cache: LRUCache<number>
  }): Promise<boolean> {
    let cacheKey = options.value
    if (options.cache.get(cacheKey)) {
      logger.info({
        module: 'rules/base',
        method: 'isAllowed',
        result: `${options.key}/${cacheKey} found in cache`
      })
      return true
    }
    const allowed = await this.fetchRemoteAllowList(cacheKey, options.key)
    if (allowed.total > 0) {
      logger.info({
        module: 'rules/base',
        method: 'isAllowed',
        result: `${options.key}/${cacheKey} found in remote allow-list`
      })
      if (this.event.test) {
        cacheKey = `${cacheKey}|${this.event.scanID}`
      }
      options.cache.set(cacheKey, 1)
      return true
    }
    return false
  }

  /**
   * wasSeen
   *
   * checks if key/value pair has already been seen in local and remote caches
   *
   * scopes test scans by scanID in local cache
   *
   * wrapper around `fetchSeenStrings` and `bumpRemoteCache`
   */
  async wasSeen(options: {
    value: string
    key: string
    cache: LRUCache<number>
  }): Promise<StoreTypeResponse> {
    let seenString = options.value
    if (this.event.test) {
      seenString = `${seenString}|${this.event.scanID}`
    }
    if (options.cache.get(options.value) === 1) {
      logger.info({
        module: 'rules/base',
        method: 'wasSeen',
        result: `${options.key}/${seenString} found in cache`
      })
      return { store: 'local' }
    }
    let seenData: StoreTypeResponse
    if (this.event.test) {
      if (options.cache.get(seenString) === 1) {
        logger.info({
          module: 'rules/base',
          method: 'wasSeen/test',
          result: `${options.key}/${seenString} found in test cache`
        })
        return { store: 'local' }
      }
      // do not update remote cache for tests
      seenData = await this.fetchSeenStrings(options.value, options.key)
    } else {
      // Check remote cache and update (read-through)
      seenData = await this.bumpRemoteCache(options.value, options.key)
    }
    options.cache.set(seenString, 1)
    return seenData
  }
}
