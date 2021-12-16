import ModelFactory from './base'
import { User, UserAttributes } from '../../models'

export default new ModelFactory<User, UserAttributes>(
  {
    login: 'admin',
    password: 'notarealpassword',
    password_hash: 'notarealhash',
    role: 'admin',
    created_at: new Date(),
  },
  User
)
