import MerryMaker, { WebRequestEvent } from '@merrymaker/types'
import Chance from 'chance'
import nock from 'nock'
import { config } from 'node-config-ts'

import unknownDomainRule, {
  domainAllowListCache,
  seenDomainCache
} from '../rules/unknown-domain'

const chance = new Chance()

describe('Unknown Domain Rule', () => {
  afterEach(() => {
    nock.cleanAll()
  })
  describe('unknown domain', () => {
    let result: MerryMaker.RuleAlert[]
    beforeAll(async () => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 0 })
      nock(config.transport.http)
        .post('/api/seen_strings/_cache', {
          seen_string: {
            key: 'www.testsite.test',
            type: 'domain'
          }
        })
        .reply(200, { store: 'none' })
      result = await unknownDomainRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })
    })
    it('alerts on unknown domain', async () => {
      expect(result[0].alert).toEqual(true)
    })
    it('updates seen_strings cache', () => {
      const seen = seenDomainCache.get('www.testsite.test')
      expect(seen).toEqual(1)
    })
  })

  describe('known domain', () => {
    beforeEach(() => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
    })

    it('does not alert on known domain in remote seen_strings', async () => {
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 0 })
      nock(config.transport.http)
        .post('/api/seen_strings/_cache', {
          seen_string: {
            key: 'www.testsite.test',
            type: 'domain'
          }
        })
        .reply(200, { store: 'database' })

      const result = await unknownDomainRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })

      expect(result[0].alert).toEqual(false)
    })
    it('does not alert on domain in remote allow list', async () => {
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 1 })
      const result = await unknownDomainRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })
      expect(result[0].alert).toEqual(false)
    })
    it('does not alert on domain in local allow list cache', async () => {
      domainAllowListCache.set('testsite.test', 1)
      const result = await unknownDomainRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })
      expect(result[0].alert).toEqual(false)
    })
    it('does not alert on domain in local seen_strings cache', async () => {
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 0 })
      seenDomainCache.set('www.testsite.test', 1)
      const result = await unknownDomainRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })
      expect(result[0].alert).toEqual(false)
    })

    describe('referrer', () => {
      const event: WebRequestEvent = {
        url: 'https://www.testsite.test',
        method: 'GET',
        resourceType: 'application/html',
        postData: '',
        response: {
          headers: {},
          status: 200,
          url: 'https://www.testsite.test'
        },
        headers: {
          referer: 'https://www.allowtest.test'
        }
      }
      beforeEach(() => {
        nock(config.transport.http)
          .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
          .reply(200, { total: 0 })
        nock(config.transport.http)
          .post('/api/seen_strings/_cache', {
            seen_string: {
              key: 'www.testsite.test',
              type: 'domain'
            }
          })
          .reply(200, { store: 'none' })
      })
      it('does not alert on allowed referrer', async () => {
        nock(config.transport.http)
          .get('/api/allow_list/?key=allowtest.test&type=referrer&field=key')
          .reply(200, { total: 1 })
        const result = await unknownDomainRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: event
        })
        expect(result[0].alert).toEqual(false)
      })
      it('updates cache', async () => {
        nock(config.transport.http)
          .get('/api/allow_list/?key=allowtest.test&type=referrer&field=key')
          .reply(200, { total: 1 })
        await unknownDomainRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: event
        })
        expect(
          domainAllowListCache.get('testsite.test|allowtest.test')
        ).toEqual(1)
      })
      it('alerts on non allowed', async () => {
        nock(config.transport.http)
          .get('/api/allow_list/?key=allowtest.test&type=referrer&field=key')
          .reply(200, { total: 0 })
        const result = await unknownDomainRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: event
        })
        expect(result[0].alert).toEqual(true)
      })
    })
  })

  describe('error handling', () => {
    beforeEach(() => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
    })
    it('handles invalid allow_list API response', async () => {
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { foo: 0 })
      let err: Error
      try {
        await unknownDomainRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: {
            url: 'https://www.testsite.test'
          } as WebRequestEvent
        })
      } catch (e) {
        err = e
      }
      expect(err).not.toBeUndefined()
    })
    it('handles invalid seen strings API response ', async () => {
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 0 })

      nock(config.transport.http)
        .post('/api/seen_strings/_cache', {
          seen_string: {
            key: 'www.testsite.test',
            type: 'domain'
          }
        })
        .reply(200, { foo: 'database' })

      let err: Error

      try {
        await unknownDomainRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: {
            url: 'https://www.testsite.test'
          } as WebRequestEvent
        })
      } catch (e) {
        err = e
      }
      expect(err).not.toBeUndefined()
    })
  })
  describe('test scans', () => {
    let result: MerryMaker.RuleAlert[]
    const scanID = chance.guid()

    beforeAll(async () => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 0 })
      nock(config.transport.http)
        // note the GET instead of POST here
        .get('/api/seen_strings/_cache')
        .query({
          key: 'www.testsite.test',
          type: 'domain'
        })
        .reply(200, { store: 'none' })
      result = await unknownDomainRule.process({
        scanID,
        test: true,
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })
    })
    it('should alert', () => {
      expect(result[0].alert).toEqual(true)
    })
    it('should scope seen_strings cache', () => {
      expect(seenDomainCache.get(`www.testsite.test|${scanID}`)).toEqual(1)
    })

    it('should not alert on seen domain during test', async () => {
      nock(config.transport.http)
        .get('/api/allow_list/?key=testsite.test&type=fqdn&field=key')
        .reply(200, { total: 0 })
      const res2 = await unknownDomainRule.process({
        scanID,
        test: true,
        type: 'request',
        payload: {
          url: 'https://www.testsite.test'
        } as WebRequestEvent
      })
      expect(res2[0].alert).toEqual(false)
    })
  })
})
