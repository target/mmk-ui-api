import { resetDB } from './utils'
import SeenStringFactory from './factories/seen_strings.factory'
import SeenStringService from '../services/seen_string'

describe('Seen String Service', () => {
  beforeEach(async () => {
    await resetDB()
  })
  describe('create', () => {
    it('creates a seen string', async () => {
      const res = await SeenStringService.create({
        type: 'fqdn',
        key: 'example.com',
      })
      expect(res.key).toBe('example.com')
    })
  })
  describe('view', () => {
    it('returns a seen string', async () => {
      const seenString = await SeenStringFactory.build({ key: 'cow' })
        .$query()
        .insert()
      const res = await SeenStringService.view(seenString.id)
      expect(res.key).toBe('cow')
    })
  })
  describe('update', () => {
    it('updates a seen string', async () => {
      const seenString = await SeenStringFactory.build({ key: 'cow' })
        .$query()
        .insert()
      const res = await SeenStringService.update(seenString.id, { key: 'moo' })
      expect(res.key).toBe('moo')
    })
  })
  describe('destroy', () => {
    it('deletes a seen string', async () => {
      const seenString = await SeenStringFactory.build({ key: 'cow' })
        .$query()
        .insert()
      const res = await SeenStringService.destroy(seenString.id)
      expect(res).toBe(1)
    })
  })
})
