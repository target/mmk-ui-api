import MerryMaker from '@merrymaker/types'
import Chance from 'chance'
import nock from 'nock'
import { config } from 'node-config-ts'

import websocketRule, {
  domainAllowListCache,
  seenDomainCache,
  iocDomainCache
} from '../rules/websocket'

const chance = new Chance()

const nockAllowList = (key: string, type: string, total: number) => {
  nock(config.transport.http)
    .get(`/api/allow_list/?key=${key}&type=${type}&field=key`)
    .reply(200, { total })
}

const nockIOCs = (type: string, value: string, total: number) => {
  nock(config.transport.http)
    .get(`/api/iocs/?type=${type}&value=${value}`)
    .reply(200, { total })
}

const nockBumpSeenStrings = (key: string, type: string, store: string) => {
  nock(config.transport.http)
    .post('/api/seen_strings/_cache', {
      seen_string: { key, type }
    })
    .reply(200, { store })
}

const nockGetSeenStrings = (key: string, type: string, store: string) => {
  nock(config.transport.http)
    .get('/api/seen_strings/_cache')
    .query({
      key,
      type
    })
    .reply(200, { store })
}

describe('Websocket Rule', () => {
  afterEach(() => {
    nock.cleanAll()
  })
  describe('unknown domain', () => {
    let result: MerryMaker.RuleAlert[]
    beforeAll(async () => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
      iocDomainCache.clear()
      nockAllowList('testsite.test', 'fqdn', 0)
      nockIOCs('fqdn', 'www.testsite.test', 0)
      nockBumpSeenStrings('www.testsite.test', 'domain', 'none')
      result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'function-call',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
    })
    it('alerts on unknown websocket domain', () => {
      expect(result[0].alert).toEqual(true)
    })
    it('updates seen domain cache', () => {
      const seen = seenDomainCache.get('www.testsite.test')
      expect(seen).toEqual(1)
    })
  })
  describe('known domain', () => {
    beforeEach(() => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
    })
    it('does not alert on known domain in remote seen_string', async () => {
      nockAllowList('testsite.test', 'fqdn', 0)
      nockIOCs('fqdn', 'www.testsite.test', 0)
      nockBumpSeenStrings('www.testsite.test', 'domain', 'database')
      const result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      expect(result[0].alert).toEqual(false)
    })
    it('does not alert on domain in remote allow list', async () => {
      nockAllowList('testsite.test', 'fqdn', 1)
      const result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      expect(result[0].alert).toEqual(false)
    })
    it('does not alert on domain in local allow list cache', async () => {
      domainAllowListCache.set('testsite.test', 1)
      const result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      expect(result[0].alert).toEqual(false)
    })
    it('does not alert on domain in local seen_strings cache', async () => {
      nockAllowList('testsite.test', 'fqdn', 0)
      nockIOCs('fqdn', 'www.testsite.test', 0)
      seenDomainCache.set('www.testsite.test', 1)
      const result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      expect(result[0].alert).toEqual(false)
    })
  })
  describe('ioc domain', () => {
    beforeEach(() => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
      iocDomainCache.clear()
    })
    it('alerts on ioc domain', async () => {
      nockAllowList('testsite.test', 'fqdn', 0)
      nockIOCs('fqdn', 'www.testsite.test', 1)
      const result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      expect(result[0].alert).toEqual(true)
    })
    it('updates ioc domain cache', async () => {
      nockAllowList('testsite.test', 'fqdn', 0)
      nockIOCs('fqdn', 'www.testsite.test', 1)
      await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      const seen = iocDomainCache.get('www.testsite.test')
      expect(seen).toEqual(1)
    })
    it('alerts on ioc domain from cache', async () => {
      nockAllowList('testsite.test', 'fqdn', 0)
      iocDomainCache.set('www.testsite.test', 1)
      const result = await websocketRule.process({
        scanID: chance.guid(),
        type: 'request',
        payload: {
          func: 'websocket',
          args: 'ws://www.testsite.test:8080'
        } as MerryMaker.WebFunctionCallEvent
      })
      expect(result[0].alert).toEqual(true)
    })
  })

  describe('error handling', () => {
    beforeEach(() => {
      domainAllowListCache.clear()
      seenDomainCache.clear()
      iocDomainCache.clear()
    })
    it('handles unexpected arguments', async () => {
      let err: Error
      let result: MerryMaker.RuleAlert[]
      try {
        result = await websocketRule.process({
          scanID: chance.guid(),
          type: 'request',
          payload: {
            func: 'websocket',
            args: 'test-args'
          } as MerryMaker.WebFunctionCallEvent
        })
      } catch (e) {
        err = e
      }
      expect(err).toBeUndefined()
      expect(result[0].message).toContain('missing / empty domain for payload')
    })
  })

  describe('test scans', () => {
    describe('unknown domain', () => {
      let result: MerryMaker.RuleAlert[]
      const scanID = chance.guid()

      beforeAll(async () => {
        domainAllowListCache.clear()
        seenDomainCache.clear()
        iocDomainCache.clear()

        nockAllowList('testsite.test', 'fqdn', 0)
        nockIOCs('fqdn', 'www.testsite.test', 0)
        nockGetSeenStrings('www.testsite.test', 'domain', 'none')

        result = await websocketRule.process({
          scanID,
          type: 'function-call',
          test: true,
          payload: {
            func: 'websocket',
            args: 'ws://www.testsite.test:8080'
          } as MerryMaker.WebFunctionCallEvent
        })
      })
      it('should alert', () => {
        expect(result[0].alert).toEqual(true)
      })
      it('should scope seen_strings cache', () => {
        expect(seenDomainCache.get(`www.testsite.test|${scanID}`)).toEqual(1)
      })
      it('should not alert on seen string during test', async () => {
        nockAllowList('testsite.test', 'fqdn', 0)
        nockIOCs('fqdn', 'www.testsite.test', 0)
        const res2 = await websocketRule.process({
          scanID,
          type: 'function-call',
          test: true,
          payload: {
            func: 'websocket',
            args: 'ws://www.testsite.test:8080'
          } as MerryMaker.WebFunctionCallEvent
        })
        expect(res2[0].alert).toEqual(false)
      })
    })
    describe('ioc domain', () => {
      let result: MerryMaker.RuleAlert[]
      const scanID = chance.guid()
      beforeAll(async () => {
        domainAllowListCache.clear()
        seenDomainCache.clear()
        iocDomainCache.clear()

        nockAllowList('testsite.test', 'fqdn', 0)
        nockIOCs('fqdn', 'www.testsite.test', 1)
        result = await websocketRule.process({
          scanID,
          type: 'function-call',
          test: true,
          payload: {
            func: 'websocket',
            args: 'ws://www.testsite.test:8080'
          } as MerryMaker.WebFunctionCallEvent
        })
      })
      it('should alert', () => {
        expect(result[0].alert).toEqual(true)
      })
      it('should scope ioc cache', () => {
        expect(iocDomainCache.get(`www.testsite.test|${scanID}`)).toEqual(1)
      })
    })
  })
})
