import { PathItem, ajv } from 'aejo'
import request from 'supertest'
import { AllowList, knex } from '../models'
import { cache } from '../services/allow_list'
import AllowListFactory from './factories/allow_list.factory'
import { makeSession, guestSession, resetDB } from './utils'
import { uuidFormat } from '../api/crud/schemas'
import { redisClient } from '../repos/redis'

const userSession = () =>
  makeSession({
    firstName: 'User',
    lastName: 'User',
    role: 'user',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
    exp: 1,
  })

const transportSession = () =>
  makeSession({
    firstName: 'Transport',
    lastName: 'user',
    role: 'transport',
    lanid: 'transport',
    email: 'transport@example.com',
    isAuth: true,
    exp: 1,
  })

const adminSession = () =>
  makeSession({
    firstName: 'Admin',
    lastName: 'User',
    role: 'admin',
    exp: 0,
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
  })

const cache_key = 'allow_list:fqdn:example.com'
const cacheQuery = {
  key: 'example.com',
  type: 'fqdn',
  field: 'key',
}

describe('AllowList Controller', () => {
  let seed: AllowList
  let api: PathItem
  // let app: Express
  beforeAll(() => {
    api = adminSession().paths
  })
  beforeEach(async () => {
    await resetDB()
    seed = await AllowListFactory.build().$query().insert().returning('*')
  })
  afterAll(async () => knex.destroy)
  describe('GET /api/allow_list', () => {
    it('should get a list of all AllowLists', async () => {
      const res = await request(userSession().app).get('/api/allow_list')
      expect(res.status).toBe(200)
      expect(res.body.results[0].key).toBe(seed.key)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/allow_list')
      expect(res.status).toBe(401)
    })
    it('should return fqdn match', async () => {
      await AllowListFactory.build({ type: 'fqdn', key: 'example.com' })
        .$query()
        .insert()
        .returning('*')
      const res = await request(userSession().app)
        .get('/api/allow_list')
        .query({ type: 'fqdn', key: 'example.com' })
      expect(res.status).toBe(200)
      expect(res.body.results[0].key).toBe('example.com')
    })
    it('should return empty on none match', async () => {
      const res = await request(userSession().app)
        .get('/api/allow_list')
        .query({ type: 'fqdn', key: 'foo123.com' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(0)
    })
    it('should match on regex', async () => {
      await AllowListFactory.build({ type: 'fqdn', key: '.*.google.com' })
        .$query()
        .insert()
        .returning('*')
      const res = await request(userSession().app)
        .get('/api/allow_list')
        .query({ type: 'fqdn', key: 'foo.google.com' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].key).toBe('.*.google.com')
    })
  })
  describe('GET /api/allow_list/:id', () => {
    it('should return an AllowList by id', async () => {
      const res = await request(adminSession().app).get(
        `/api/allow_list/${seed.id}`
      )
      expect(res.body.id).toBe(seed.id)

      // validate response OAS object
      const validate = ajv.compile(
        api[`/api/allow_list/:id(${uuidFormat})`].get.responses['200'].content[
          'application/json'
        ].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return 404 for invalid ID', async () => {
      const res = await request(adminSession().app).get(
        '/api/allow_list/3912eb36-3de6-46d1-95fe-99a4a5e80f17'
      )
      expect(res.status).toBe(404)
    })
  })
  describe('POST /api/allow_list', () => {
    it('should return from the DB', async () => {
      const res = await request(adminSession().app)
        .post('/api/allow_list')
        .send({
          allow_list: {
            key: 'example.com',
            type: 'fqdn',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.key).toBe('example.com')
    })
    it('should prevent invalid regular expression keys', async () => {
      const res = await request(adminSession().app)
        .post('/api/allow_list')
        .send({
          allow_list: {
            key: '*.bar',
            type: 'fqdn',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
    it('should prevent passing additional properties', async () => {
      const res = await request(adminSession().app)
        .post('/api/allow_list')
        .send({
          allow_list: {
            key: 'example.com',
            type: 'fqdn',
            created_at: new Date(),
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
      expect(res.body.type).toBe('ValidationError')
    })
  })
  describe('PUT /api/allow_list', () => {
    it('should return from the DB', async () => {
      const res = await request(adminSession().app)
        .put(`/api/allow_list/${seed.id}`)
        .send({
          allow_list: {
            key: 'moocar.com',
            type: 'fqdn',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.key).toBe('moocar.com')
    })
    it('should prevent invalid regular expression keys', async () => {
      const res = await request(adminSession().app)
        .put(`/api/allow_list/${seed.id}`)
        .send({
          allow_list: {
            key: '*.bar',
            type: 'fqdn',
          },
        })
        .set('Accept', 'application/json')
      const validate = ajv.compile(
        api[`/api/allow_list/:id(${uuidFormat})`].put.responses['422'].content[
          'application/json'
        ].schema
      )
      validate(res.body)
      expect(validate.errors).toBeNull()
      expect(res.status).toBe(422)
    })
  })
  describe('DELETE /api/allow_list/:id', () => {
    it('should return number of changed', async () => {
      const res = await request(adminSession().app)
        .delete(`/api/allow_list/${seed.id}`)
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
    })
    it('should not allow non-admin to delete', async () => {
      const res = await request(userSession().app)
        .delete(`/api/allow_list/${seed.id}`)
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
  })
  describe('GET /api/allow_list/_cache', () => {
    beforeEach(async () => {
      await redisClient.del(cache_key)
      cache.clear()
    })
    it('should return true/local when found in LRUcache', async () => {
      cache.set(cache_key, 1)
      const res = await request(transportSession().app)
        .get('/api/allow_list/_cache')
        .query(cacheQuery)
      expect(res.body).toEqual({ has: true, store: 'local' })
    })
    it('should return false/none when not cached', async () => {
      const res = await request(transportSession().app)
        .get('/api/allow_list/_cache')
        .query(cacheQuery)
      expect(res.body).toEqual({ has: false, store: 'none' })
    })
    it('should return validation error on missing property', async () => {
      const res = await request(transportSession().app)
        .get('/api/allow_list/_cache')
        .query({ foo: 'bar ' })
      expect(res.status).toBe(422)
    })
  })
})
