import request from 'supertest'
import { knex } from '../models'
import Ioc from '../models/iocs'
import IocFactory from './factories/iocs.factory'
import { makeSession, guestSession, resetDB } from './utils'

import Chance from 'chance'
import { redisClient } from '../repos/redis'
import { cache } from '../services/ioc'
const chance = new Chance()

const cache_key = 'iocs:fqdn:example.com'
const cacheQuery = {
  key: 'example.com',
  type: 'fqdn',
}

const userSession = () =>
  makeSession({
    firstName: 'User',
    lastName: 'User',
    role: 'user',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
    exp: 0,
  }).app

const adminSession = () =>
  makeSession({
    firstName: 'admin',
    lastName: 'User',
    exp: 0,
    role: 'admin',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
  }).app

const transportSession = () =>
  makeSession({
    firstName: 'Transport',
    lastName: 'user',
    role: 'transport',
    lanid: 'transport',
    email: 'transport@example.com',
    isAuth: true,
    exp: 1,
  }).app

describe('IOCs Controller', () => {
  let seed: Ioc
  beforeEach(async () => {
    await resetDB()
    seed = await IocFactory.build().$query().insert()
  })
  afterAll(async () => {
    knex.destroy()
  })
  describe('GET /api/iocs', () => {
    it('should get a list of Iocs', async () => {
      const res = await request(userSession()).get('/api/iocs')
      expect(res.status).toBe(200)
      expect(res.body.results[0].id).toBe(seed.id)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/iocs')
      expect(res.status).toBe(401)
    })
    it('should return fqdn match', async () => {
      await IocFactory.build({ type: 'fqdn', value: 'example.com' })
        .$query()
        .insert()
        .returning('*')

      const res = await request(userSession())
        .get('/api/iocs')
        .query({ type: 'fqdn', value: 'example.com' })
      expect(res.status).toBe(200)
      expect(res.body.results[0].value).toBe('example.com')
    })
    it('should match on regex', async () => {
      await IocFactory.build({ type: 'fqdn', value: '.*.google.com' })
        .$query()
        .insert()
        .returning('*')
      const res = await request(userSession())
        .get('/api/iocs')
        .query({ type: 'fqdn', value: 'foo.google.com' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].value).toBe('.*.google.com')
    })
  })
  describe('GET /api/iocs/:id', () => {
    it('should return the ioc for auth user', async () => {
      const res = await request(userSession()).get(`/api/iocs/${seed.id}`)
      expect(res.status).toBe(200)
      expect(res.body.value).toBe(seed.value)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get(`/api/iocs/${seed.id}`)
      expect(res.status).toBe(401)
    })
    it('should return 404 for invalid id', async () => {
      const res = await request(userSession()).get(
        `/api/iocs/${chance.guid({ version: 4 })}`
      )
      expect(res.status).toBe(404)
    })
  })
  describe('POST /api/iocs', () => {
    it('should create IOC for admin user', async () => {
      const newIOC: Ioc = IocFactory.build()
      const ioc = newIOC.toJSON()
      const res = await request(adminSession())
        .post('/api/iocs')
        .send({
          ioc: { type: ioc.type, value: ioc.value, enabled: ioc.enabled },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.type).toBe(newIOC.type)
    })
    it('should reject create IOC for normal user', async () => {
      const newIOC: Ioc = IocFactory.build()
      const ioc = newIOC.toJSON()
      const res = await request(userSession())
        .post('/api/iocs')
        .send({
          ioc: { type: ioc.type, value: ioc.value, enabled: ioc.enabled },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
    it('should reject invalid regular expression', async () => {
      const newIOC: Ioc = IocFactory.build({ value: '*.bar' })
      const ioc = newIOC.toJSON()
      const res = await request(adminSession())
        .post('/api/iocs')
        .send({
          ioc: { type: ioc.type, value: ioc.value, enabled: ioc.enabled },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
    it('should reject on empty value', async () => {
      const res = await request(adminSession())
        .post('/api/iocs')
        .send({ ioc: { value: '', type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(400)
    })
  })
  describe('POST /api/iocs/bulk', () => {
    it('should create IOCs in bulk for admin user', async () => {
      const values: string[] = [
        IocFactory.build().value,
        IocFactory.build().value,
      ]
      const res = await request(adminSession())
        .post('/api/iocs/bulk')
        .send({ iocs: { values, type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.message).toBe('created')
    })
    it('should throw validation error on empty array', async () => {
      const res = await request(adminSession())
        .post('/api/iocs/bulk')
        .send({ iocs: { values: [], type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
    it('should throw validation error on invalid value', async () => {
      const res = await request(adminSession())
        .post('/api/iocs/bulk')
        .send({ iocs: { values: ['*.bar'], type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
  })
  describe('PUT /api/iocs', () => {
    it('should update IOC for admin user', async () => {
      const res = await request(adminSession())
        .put(`/api/iocs/${seed.id}`)
        .send({ ioc: { value: 'example.com', type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.value).toBe('example.com')
    })
    it('should reject update for normal user', async () => {
      const res = await request(userSession())
        .put(`/api/iocs/${seed.id}`)
        .send({ ioc: { value: 'example.com', type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
    it('should prevent invalid regular expression value', async () => {
      const res = await request(adminSession())
        .put(`/api/iocs/${seed.id}`)
        .send({ ioc: { value: '*.bar', type: 'fqdn', enabled: true } })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
  })
  describe('DELETE /api/iocs', () => {
    it('should delete IOC for admin user', async () => {
      const res = await request(adminSession()).delete(`/api/iocs/${seed.id}`)
      expect(res.status).toBe(200)
    })
    it('should reject delete IOC for normal user', async () => {
      const res = await request(userSession()).delete(`/api/iocs/${seed.id}`)
      expect(res.status).toBe(403)
    })
  })
  describe('GET /api/iocs/_cache', () => {
    beforeEach(async () => {
      await redisClient.del(cache_key)
      cache.clear()
    })
    it('should return true/local when found in LRUcache', async () => {
      cache.set(cache_key, 1)
      const res = await request(transportSession())
        .get('/api/iocs/_cache')
        .query(cacheQuery)
      expect(res.body).toEqual({ has: true, store: 'local' })
    })
    it('should return false/none when not cached', async () => {
      const res = await request(transportSession())
        .get('/api/iocs/_cache')
        .query(cacheQuery)
      expect(res.body).toEqual({ has: false, store: 'none' })
    })
    it('should return true/database when found in database', async () => {
      const inDB = await IocFactory.build().$query().insert()
      const res = await request(transportSession())
        .get('/api/iocs/_cache')
        .query({ key: inDB.value, type: inDB.type })
      expect(res.body).toEqual({ has: true, store: 'database' })
    })
  })
})
