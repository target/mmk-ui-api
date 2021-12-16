import request from 'supertest'
import { knex } from '../models'
import { guestSession, makeSession, resetDB } from './utils'
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

describe('Auth Controller', () => {
  beforeEach(async () => {
    await resetDB()
  })
  afterAll(async () => knex.destroy)

  describe('POST /api/auth/login', function () {
    beforeEach(async () =>
      UserFactory.build({ password: 'not-a-real-password' }).$query().insert()
    )
    it('should login user with valid creds', async () => {
      const res = await request(guestSession().app)
        .post('/api/auth/login')
        .send({ user: { login: 'admin', password: 'not-a-real-password' } })
      expect(res.status).toBe(200)
      expect(res.body.login).toBe('admin')
    })
    it('should reject invalid login creds', async () => {
      const res = await request(guestSession().app)
        .post('/api/auth/login')
        .send({ user: { login: 'admin', password: 'the-wrong-password' } })
      expect(res.status).toBe(401)
    })
    it('should reject on invalid request', async () => {
      const res = await request(guestSession().app)
        .post('/api/auth/login')
        .send({ user: { username: 'admin', password: 'the-wrong-password' } })
      expect(res.status).toBe(422)
    })
  })
  describe('GET /api/auth/logout', function () {
    it('should log out the user', async () => {
      const res = await request(userSession().app).get('/api/auth/logout')
      expect(res.status).toBe(200)
    })
    it('should reject logout for guest session', async () => {
      const res = await request(guestSession().app).get('/api/auth/logout')
      expect(res.status).toBe(401)
    })
  })
  describe('GET /api/auth/ready', function () {
    it('should return local/false for missing admin', async () => {
      const res = await request(guestSession().app).get('/api/auth/ready')
      expect(res.status).toBe(200)
      expect(res.body).toEqual({ ready: false, strategy: 'local' })
    })
    it('should return local/true for found admin', async () => {
      await UserFactory.build({
        login: 'admin',
        password: 'not-a-real-password',
      })
        .$query()
        .insert()
      const res = await request(guestSession().app).get('/api/auth/ready')
      expect(res.status).toBe(200)
      expect(res.body).toEqual({ ready: true, strategy: 'local' })
    })
  })
})
