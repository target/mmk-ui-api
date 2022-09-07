import MerryMaker, { WebRequestEvent } from '@merrymaker/types'
import path from 'path'
import Chance from 'chance'
import { config } from 'node-config-ts'
import nock from 'nock'
import YaraSync from '../lib/yara-sync'

const chance = new Chance()

import iocPayloadRule, {
  payloadAllowListCache,
  iocPayloadCache
} from '../rules/ioc.payload'

const yara = new YaraSync()

const testCases = [
  [
    'ioc_payload_checkout_clear_cc',
    'https://example.com?cc=4111123412341234',
    1,
  ],
  [
    'ioc_payload_checkout_b64_cc',
    'https://example.com?cc=NDExMTEyMzQxMjM0MTIzNA',
    1,
  ],
  [
    'fetch_abnormal_content',
    '"resourceType":"fetch" "content-type":"image',
    1,
  ],
]

describe('IOC Payload Rules', () => {
  beforeAll(async () => {
    try {
      await yara.initAsync({
        rules: [
          {
            filename: path.resolve(__dirname, '../rules', 'ioc.payloads.yara'),
          },
        ],
      })
    } catch (e) {
      console.log('error', e)
      throw e
    }
  })

  test.each(testCases)(
    'detects %p as %p',
    async (expectedID, sample, expectedMatches) => {
      const result = await yara.scanAsync({
        buffer: Buffer.from(sample as string, 'utf-8'),
      })
      if (expectedID) {
        expect(result.rules[0].id).toBe(expectedID)
      }
      expect(result.rules.length).toBe(expectedMatches)
    }
  )

  describe('process', () => {
    afterEach(() => {
      nock.cleanAll()
    })
    describe('ioc payload', () => {
      let result: MerryMaker.RuleAlert[]
      beforeAll(async () => {
        payloadAllowListCache.clear()
        iocPayloadCache.clear()
        nock(config.transport.http)
          .get(
            '/api/allow_list/?key=www.testsite.test&type=ioc-payload-domain&field=key'
          )
          .reply(200, { total: 0 })
        result = await iocPayloadRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: {
            url: 'https://www.testsite.test?cc=NDExMTEyMzQxMjM0MTIzNA'
          } as WebRequestEvent
        })
      })
      it('alerts on ioc payload', () => {
        expect(result[0].alert).toEqual(true)
      })
      it('updates iocPayloadCache', () => {
        const seen = iocPayloadCache.get(iocPayloadRule.combinedHash)
        expect(seen).toEqual(1)
      })
    })
    describe('allowed ioc payload', () => {
      it('does not alert', async () => {
        // move to beforeEach as needed
        payloadAllowListCache.clear()
        iocPayloadCache.clear()
        nock(config.transport.http)
          .get(
            '/api/allow_list/?key=www.testsite.test&type=ioc-payload-domain&field=key'
          )
          .reply(200, { total: 1 })
        const result = await iocPayloadRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: {
            url: 'https://www.testsite.test?cc=NDM5NDA4NDMyOTA4OTA1'
          } as WebRequestEvent
        })
        expect(result[0].alert).toEqual(false)
      })
    })
    describe('test scans', () => {
      let result: MerryMaker.RuleAlert[]
      const scanID = chance.guid()
      beforeAll(async () => {
        payloadAllowListCache.clear()
        iocPayloadCache.clear()
        nock(config.transport.http)
          .get(
            '/api/allow_list/?key=www.testsite.test&type=ioc-payload-domain&field=key'
          )
          .reply(200, { total: 0 })
        result = await iocPayloadRule.process({
          scanID,
          test: true,
          type: 'request',
          payload: {
            url: 'https://www.testsite.test?cc=NDExMTEyMzQxMjM0MTIzNA'
          } as WebRequestEvent
        })
      })
      it('should alert', () => {
        expect(result[0].alert).toEqual(true)
      })
      it('should scope iocPayloadCache cache', () => {
        expect(
          iocPayloadCache.get(`${iocPayloadRule.combinedHash}|${scanID}`)
        ).toEqual(1)
      })
      it('should not alert on seen hash during test', async () => {
        nock(config.transport.http)
          .get(
            '/api/allow_list/?key=www.testsite.test&type=ioc-payload-domain&field=key'
          )
          .reply(200, { total: 0 })
        const res2 = await iocPayloadRule.process({
          scanID,
          test: true,
          type: 'request',
          payload: {
            url: 'https://www.testsite.test?cc=NDExMTEyMzQxMjM0MTIzNA'
          } as WebRequestEvent
        })
        expect(res2[0].alert).toEqual(false)
      })
    })
  })
})
