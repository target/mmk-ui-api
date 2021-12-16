import MerryMakerTypes from '@merrymaker/types'
import { config } from 'node-config-ts'

import fetch from 'node-fetch'

const allowListURL = `${config.transport.http}/api/allow_list`

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
    return allow.json()
  }

  async fetchRemoteIOC(hostname: string): Promise<{ total: number }> {
    const remoteIOC = await fetch(
      `${config.transport.http}/api/iocs/?type=fqdn&value=${hostname}`
    )
    return remoteIOC.json()
  }

  async addRemoteAllowList(
    key: string,
    type: string
  ): Promise<{ store: number }> {
    const addReq = await fetch(allowListURL, {
      method: 'post',
      body: JSON.stringify({
        // eslint-disable-next-line @typescript-eslint/camelcase
        allow_list: {
          key,
          type,
        },
      }),
      headers: { 'Content-Type': 'application/json' },
    })
    return addReq.json()
  }

  async bumpRemoteCache(key: string, type: string): Promise<{ store: string }> {
    // bump remote cache
    const seenReq = await fetch(
      `${config.transport.http}/api/seen_strings/_cache`,
      {
        method: 'post',
        body: JSON.stringify({
          // eslint-disable-next-line @typescript-eslint/camelcase
          seen_string: {
            key,
            type,
          },
        }),
        headers: { 'Content-Type': 'application/json' },
      }
    )
    return seenReq.json()
  }
}
