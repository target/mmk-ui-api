import MerryMakerTypes from '@merrymaker/types'
import { config } from 'node-config'

import fetch from 'node-fetch-cjs'
import { isOfType } from '../lib/utils'

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
  alertResults: MerryMakerTypes.RuleAlert[]
  constructor(protected readonly options: MerryMakerTypes.RuleAlert) {}
  abstract process(
    evt: MerryMakerTypes.ScanEventPayload
  ): Promise<MerryMakerTypes.RuleAlert[]>
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
}
