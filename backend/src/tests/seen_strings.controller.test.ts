import request from 'supertest'
import { SeenString, knex } from '../models'
import { cache } from '../services/seen_string'
import { redisClient } from '../repos/redis'
import SeenStringFactory from './factories/seen_strings.factory'
import { makeSession, guestSession, resetDB } from './utils'

const adminSession = () =>
  makeSession({
    firstName: 'Admin',
    lastName: 'User',
    role: 'admin',
    exp: 0,
    lanid: 'z000n00',
    email: 'example@example.com',
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

const cache_key = 'seen_strings:fqdn:example.com'
const cacheQuery = {
  key: 'example.com',
  type: 'fqdn',
}

describe('SeenStrings Controller', () => {
  let seed: SeenString
  beforeEach(async () => {
    await resetDB()
    seed = await SeenStringFactory.build().$query().insert().returning('*')
  })
  afterAll(async () => knex.destroy)
  describe('GET /api/seen_strings', () => {
    it('should get a list of all SeenStrings', async () => {
      const res = await request(adminSession()).get('/api/seen_strings')
      expect(res.status).toBe(200)
      expect(res.body.results[0].key).toBe(seed.key)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/seen_strings')
      expect(res.status).toBe(401)
    })
  })
  describe('GET /api/seen_strings/:id', () => {
    it('should return a seen string', async () => {
      const res = await request(adminSession()).get(
        `/api/seen_strings/${seed.id}`
      )
      expect(res.status).toBe(200)
      expect(res.body.id).toBe(seed.id)
    })
  })
  describe('PUT /api/seen_strings/:id', () => {
    it('should update seen string', async () => {
      const res = await request(adminSession())
        .put(`/api/seen_strings/${seed.id}`)
        .send({ seen_string: { key: 'example', type: 'string' } })
      expect(res.body.key).toBe('example')
    })
    it('should reject invalid update', async () => {
      const res = await request(adminSession())
        .put(`/api/seen_strings/${seed.id}`)
        .send({ seen_string: { value: 'example' } })
      expect(res.status).toBe(422)
      expect(res.body.type).toBe('ValidationError')
    })
  })
  describe('DELETE /api/seen_strings/:id', () => {
    it('should delete seen string', async () => {
      const res = await request(adminSession()).delete(
        `/api/seen_strings/${seed.id}`
      )
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
    })
  })
  describe('GET /api/seen_strings/_cache', () => {
    beforeEach(async () => {
      await redisClient.del(cache_key)
      cache.clear()
    })
    it('should return true/local when found in LRUcache', async () => {
      cache.set(cache_key, 1)
      const res = await request(transportSession())
        .get('/api/seen_strings/_cache')
        .query(cacheQuery)
      expect(res.body).toEqual({ has: true, store: 'local' })
    })
    it('should return false/none when not cached', async () => {
      const res = await request(transportSession())
        .get('/api/seen_strings/_cache')
        .query(cacheQuery)
      expect(res.body).toEqual({ has: false, store: 'none' })
    })
    it('should return validation error on missing property', async () => {
      const res = await request(transportSession())
        .get('/api/seen_strings/_cache')
        .query({ foo: 'bar' })
      expect(res.status).toBe(422)
    })
  })
  describe('POST /api/seen_strings/_cache', () => {
    beforeEach(async () => {
      cache.clear()
    })
    it('should return from the DB', async () => {
      await redisClient.del('seen_strings:domain:example.com')
      const res = await request(adminSession())
        .post('/api/seen_strings/_cache')
        .send({
          seen_string: {
            key: 'example.com',
            type: 'domain',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.store).toBe('none')
    })
    it('should return validation error', async () => {
      const res = await request(adminSession())
        .post('/api/seen_strings/_cache')
        .send({
          seen_string: {
            key: 'example.com',
            value: 'domain',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
    it('should return from local', async () => {
      await redisClient.del('seen_strings:domain:example2.com')
      const payload = {
        seen_string: {
          key: 'example2.com',
          type: 'domain',
        },
      }
      await request(adminSession())
        .post('/api/seen_strings/_cache')
        .send(payload)
        .set('Accept', 'application/json')
      const res = await request(adminSession())
        .post('/api/seen_strings/_cache')
        .send(payload)
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.store).toBe('local')
    })
  })
})
