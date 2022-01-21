import { User, UserAttributes } from '../models'

const findOne = async (query: Partial<UserAttributes>): Promise<User> =>
  User.query().findOne(query)

const view = async (id: string): Promise<User> =>
  User.query().findById(id).throwIfNotFound()

const update = async (
  id: string,
  attrs: Partial<UserAttributes>
): Promise<User> => User.query().patchAndFetchById(id, attrs)

const create = async (attrs: Partial<UserAttributes>): Promise<User> =>
  User.query().insert(attrs)

const destroy = async (id: string): Promise<number> =>
  User.query().deleteById(id)

export default {
  view,
  findOne,
  update,
  create,
  destroy,
}
