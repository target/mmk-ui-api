import SecretService from '../services/secret'
import SourceService from '../services/source'
import SecretFactory from './factories/secrets.factory'
// import { Secret } from '../models'
import { resetDB } from './utils'

describe('Secret Service (⌐■_■)', () => {
  beforeEach(async () => {
    await resetDB()
  })
  describe('update', () => {
    it('updates secret and source cache', async () => {
      const secret = await SecretFactory.build({ name: 'cow' })
        .$query()
        .insert()
      const source = await SourceService.create({
        name: 'foobar',
        value: 'call("__cow__")',
        secret_ids: [secret.id],
      })
      await SecretService.update(secret.id, { value: 'moocar' })
      const cachedValue = await SourceService.getCache(source.id)
      expect(cachedValue).toBe('call("moocar")')
    })
  })
  describe('create', () => {
    it('creates a secret', async () => {
      const res = await SecretService.create({
        name: 'cow',
        type: 'manual',
        value: 'moo',
      })
      expect(res.name).toBe('cow')
    })
  })
  describe('view', () => {
    it('returns a secret', async () => {
      const secret = await SecretFactory.build({ name: 'cow' })
        .$query()
        .insert()
      const res = await SecretService.view(secret.id)
      expect(res.id).toBe(secret.id)
    })
    it('throws undefined if not found', async () => {
      let err: Error
      try {
        await SecretService.view('e1968d81-37d1-4d23-a2a1-2b9331488bb3')
      } catch (e) {
        err = e
      }
      expect(err).not.toBeUndefined
    })
  })
  describe('destroy', () => {
    it('deletes a secret', async () => {
      const secret = await SecretFactory.build({ name: 'cow' })
        .$query()
        .insert()
      const res = await SecretService.destroy(secret.id)
      expect(res).toBe(1)
    })
  })
  describe('isInUse', () => {
    it('returns true when in use by a source', async () => {
      const secret = await SecretFactory.build({ name: 'cow' })
        .$query()
        .insert()
      await SourceService.create({
        name: 'foobar',
        value: 'call("__cow__")',
        secret_ids: [secret.id],
      })
      const res = await SecretService.isInUse(secret.id)
      expect(res).toBe(true)
    })
    it('returns false when not in use by a source', async () => {
      const res = await SecretService.isInUse(
        'c4fafb71-7f3f-459d-ae3e-f92daf619dd6'
      )
      expect(res).toBe(false)
    })
  })
})
