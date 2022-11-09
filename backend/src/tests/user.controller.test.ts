import { PathItem, ajv } from 'aejo'
import { knex, User } from '../models'
import request from 'supertest'
import { makeSession, resetDB, guestSession } from './utils'
import UserFactory from './factories/user.factory'

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

describe('User Controller', () => {
  let api: PathItem
  let seed: User
  beforeAll(() => {
    api = userSession().paths
  })
  beforeEach(async () => {
    await resetDB()
    seed = await UserFactory.build().$query().insert()
    await UserFactory.build({ login: 'user', role: 'user' }).$query().insert()
  })
  afterAll(async () => knex.destroy)
  describe('GET /api/users', () => {
    it('should get a list of Users', async () => {
      const res = await request(adminSession().app).get('/api/users')
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(2)
      const validate = ajv.compile(
        api['/api/users/'].get.responses['200'].content['application/json']
          .schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return 401 for unauthenticated user', async () => {
      const res = await request(guestSession().app).get('/api/users')
      expect(res.status).toBe(401)
    })
    it('should return role match', async () => {
      const res = await request(adminSession().app)
        .get('/api/users')
        .query({ role: 'admin' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
      expect(res.body.results[0].role).toBe('admin')
    })
    it('should return empty on no match', async () => {
      const res = await request(adminSession().app)
        .get('/api/users')
        .query({ role: 'transport' })
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(0)
    })
    it('should return validation error on invalid role', async () => {
      const res = await request(adminSession().app)
        .get('/api/users')
        .query({ role: 'badrole' })
      expect(res.status).toBe(422)
    })
  })
  describe('GET /api/users/{id}', () => {
    it('should return a User by id', async () => {
      const res = await request(adminSession().app).get(`/api/users/${seed.id}`)
      expect(res.body.id).toBe(seed.id)
      const validate = ajv.compile(
        api['/api/users/{id}'].get.responses['200'].content[
          'application/json'
        ].schema
      )
      expect(validate(res.body)).toBe(true)
    })
    it('should return 404 for invalid ID', async () => {
      const res = await request(adminSession().app).get(
        '/api/users/3912eb36-3de6-46d1-95fe-99a4a5e80f17'
      )
      expect(res.status).toBe(404)
    })
  })
  describe('POST /api/users', () => {
    it('should return from the DB', async () => {
      const res = await request(adminSession().app)
        .post('/api/users')
        .send({
          user: {
            login: 'admin1',
            password: 'somepassword',
            role: 'admin',
          },
        })
        .set('Aceept', 'application/json')
      expect(res.status).toBe(200)
      expect(res.body.login).toBe('admin1')
    })
    it('should prevent duplicate login', async () => {
      const res = await request(adminSession().app)
        .post('/api/users')
        .send({
          user: {
            login: 'admin',
            password: 'someotherpass',
            role: 'admin',
          },
        })
        .set('Accept', 'application/json')
      expect(res.status).toBe(400)
      expect(res.body.type).toBe('ConstraintViolationError')
    })
    it('should prevent short password', async () => {
      const res = await request(adminSession().app)
        .post('/api/users')
        .send({
          user: {
            login: 'admin1',
            password: 'short',
            role: 'admin',
          },
        })
      expect(res.status).toBe(422)
      expect(res.body.type).toBe('ValidationError')
    })
  })
  describe('PUT /api/users', () => {
    it('should return from the DB', async () => {
      const res = await request(adminSession().app)
        .put(`/api/users/${seed.id}`)
        .send({
          user: {
            login: 'admin1',
            role: 'admin',
          },
        })
      expect(res.status).toBe(200)
      expect(res.body.login).toBe('admin1')
    })
    it('should prevent non-admin user', async () => {
      const res = await request(userSession().app)
        .put(`/api/users/${seed.id}`)
        .send({
          user: {
            login: 'admin1',
            role: 'admin',
          },
        })
      expect(res.status).toBe(403)
    })
  })
  describe('DELETE /api/users/:id', () => {
    it('should return number of changed', async () => {
      const res = await request(adminSession().app).delete(
        `/api/users/${seed.id}`
      )
      expect(res.status).toBe(200)
      expect(res.body.total).toBe(1)
    })
    it('should not allow non-admin to delete', async () => {
      const res = await request(userSession().app).delete(
        `/api/users/${seed.id}`
      )
      expect(res.status).toBe(403)
    })
  })
})
