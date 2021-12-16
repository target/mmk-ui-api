import request from 'supertest'
import { Secret, knex } from '../models'
import SecretFactory from './factories/secrets.factory'
import SourceFactory from './factories/sources.factory'
import SourceService from '../services/source'
import SecretService from '../services/secret'
import { resetDB, makeSession } from './utils'
import { PathItem, ajv } from 'aejo'

const userSession = () =>
  makeSession({
    firstName: 'User',
    lastName: 'User',
    role: 'user',
    lanid: 'z000n00',
    email: 'foo@example.com',
    isAuth: true,
    exp: 0,
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

describe('Secret Controller', () => {
  let seed: Secret
  let api: PathItem
  beforeAll(() => {
    api = adminSession().paths
  })
  beforeEach(async () => {
    await resetDB()
    seed = await SecretFactory.build().$query().insert()
  })
  afterAll(async () => knex.destroy)
  describe('GET /api/secrets', () => {
    it('should allow admin to fetch secrets', async () => {
      const res = await request(adminSession().app).get('/api/secrets')
      expect(res.status).toBe(200)
      expect(res.body.results[0].id).toBe(seed.id)

      const validate = ajv.compile(
        api['/api/secrets/'].get.responses['200'].content['application/json']
          .schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should not allow user to fetch secrets', async () => {
      const res = await request(userSession().app).get('/api/secrets')
      expect(res.status).toBe(403)
    })
  })
  describe('GET /api/secrets/:id', () => {
    it('should allow admin to fetch secret by ID', async () => {
      const res = await request(adminSession().app).get(
        `/api/secrets/${seed.id}`
      )
      expect(res.status).toBe(200)
      expect(res.body.id).toBe(seed.id)
    })
    it('should not allow user to fetch secret by ID', async () => {
      const res = await request(userSession().app).get(
        `/api/secrets/${seed.id}`
      )
      expect(res.status).toBe(403)
    })
  })
  describe('POST /api/secrets', () => {
    it('should create a new secret', async () => {
      const res = await request(adminSession().app)
        .post('/api/secrets')
        .send({
          secret: {
            name: 'foobar',
            value: 'moocar',
            type: 'manual',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.name).toBe('foobar')
    })
    it('should not allow user to create a secret', async () => {
      const res = await request(userSession().app)
        .post('/api/secrets')
        .send({
          name: 'foobar',
          value: 'moocar',
          type: 'manual',
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
  })
  describe('PUT /api/secrets/:id', () => {
    it('should update existing secret', async () => {
      const res = await request(adminSession().app)
        .put(`/api/secrets/${seed.id}`)
        .send({
          secret: {
            value: 'moocar',
            type: 'manual',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.value).toBe('moocar')
    })
    it('should not allow user to update secret name', async () => {
      const res = await request(adminSession().app)
        .put(`/api/secrets/${seed.id}`)
        .send({
          secret: {
            name: 'newName',
            value: 'moocar',
            type: 'manual',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(422)
    })
    it('should not allow user to update secret', async () => {
      const res = await request(userSession().app)
        .put(`/api/secrets/${seed.id}`)
        .send({
          name: 'moocar',
          value: 'moocar',
          type: 'manual',
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(403)
    })
    it('should update cached source', async () => {
      const sourceSeed = await SourceService.create({
        ...SourceFactory.build({
          value: `call("__${seed.name}__")`,
        }),
        secret_ids: [seed.id],
      })
      await SecretService.update(seed.id, {
        ...seed,
        value: 'moocar',
      })
      const value = await SourceService.getCache(sourceSeed.id)
      expect(value).toBe('call("moocar")')
    })
  })
  describe('DELETE /api/secrets/:id', () => {
    it('should delete unused secret', async () => {
      const res = await request(adminSession().app).delete(
        `/api/secrets/${seed.id}`
      )
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
    })
    it('should not allow deletion of inuse secret', async () => {
      await SourceService.create({
        ...SourceFactory.build({
          value: `call("__${seed.name}__")`,
        }),
        secret_ids: [seed.id],
      })
      const res = await request(adminSession().app).delete(
        `/api/secrets/${seed.id}`
      )
      expect(res.status).toBe(422)
      expect(res.body.message).toBe('Cannot delete active secret')
    })
    it('should not allow non-admin user to delete a secret', async () => {
      const res = await request(userSession().app).delete(
        `/api/secrets/${seed.id}`
      )
      expect(res.status).toBe(403)
    })
  })
  describe('GET /api/secrets/types', () => {
    it('should return configured types', async () => {
      const res = await request(adminSession().app).get('/api/secrets/types')
      expect(res.status).toBe(200)
      expect(res.body).toEqual({
        types: ['manual'],
      })
    })
  })
})
