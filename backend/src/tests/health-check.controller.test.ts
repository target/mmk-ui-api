import { PathItem, ajv } from 'aejo'
import request, { Response } from 'supertest'
import { makeSession, guestSession } from './utils'

describe('Health Check Controller', () => {
  describe('GET /api/health', () => {
    let api: PathItem
    let res: Response
    beforeAll(async () => {
      api = makeSession().paths
      res = await request(guestSession().app).get('/api/health')
    })
    it('should return 200', () => {
      expect(res.status).toBe(200)
    })
    it('should return valid response', () => {
      const validate = ajv.compile(
        api['/api/health'].get.responses['200'].content['text/plain'].schema
      )
      validate(res.text)
      expect(validate.errors).toBeNull()
    })
  })
})
