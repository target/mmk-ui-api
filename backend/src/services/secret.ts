import { SourceSecret, Secret, SecretAttributes } from '../models'
import SourceService from './source'

/**
 * update
 *
 * Updates a secret and related source cache with the new value
 */
const update = async (
  id: string,
  secret: Partial<SecretAttributes>
): Promise<Secret> => {
  const updated = await Secret.query().patchAndFetchById(
    id,
    Secret.updateAble().reduce(
      (obj, key) => ({ ...obj, [key]: secret[key] }),
      {}
    )
  )
  const sources = await SourceSecret.query().where({ secret_id: updated.id })
  await Promise.all(sources.map((s) => SourceService.cache(s.source_id)))
  return updated
}

const create = async (attrs: Partial<SecretAttributes>): Promise<Secret> =>
  Secret.query().insert(attrs)

const view = async (id: string): Promise<Secret> =>
  Secret.query().findById(id).throwIfNotFound()

const destroy = async (id: string): Promise<number> =>
  Secret.query().deleteById(id)

const isInUse = async (id: string): Promise<boolean> => {
  const res = await SourceSecret.query().where({ secret_id: id })
  return res && res.length > 0
}

export default {
  view,
  isInUse,
  create,
  update,
  destroy,
}
