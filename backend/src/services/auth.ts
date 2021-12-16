import { Session } from 'express-session'
import { UserAttributes, UserRole } from '../models/users'
import { User } from '../models'

const roleLevels: Record<string, number> = {
  admin: 100,
  transport: 90,
  user: 50,
}

const isAuth = (session: Session): boolean => {
  if (session.data === undefined) {
    return false
  }
  return session.data.isAuth === true
}

const isRole = (user: UserSession, role: UserRole): boolean =>
  user.role === role

const hasRole = (user: UserSession, role: UserRole): boolean =>
  roleLevels[user.role] >= roleLevels[role]

/**
 * verifyLocalCreds
 *
 * Queries `users` table fo matching login,
 * uses bcrypt compare against the password_hash to verify match
 */
const verifyLocalCreds = async (
  user: Pick<UserAttributes, 'login' | 'password'>
): Promise<{ auth: boolean; user?: User }> => {
  const instance = await User.query().findOne({ login: user.login })
  if (!instance) {
    return { auth: false }
  }
  const auth = await instance.checkPassword(user.password)
  return { auth, user: instance }
}

/**
 * buildSession
 *
 * Creates a UserSession from a User instance
 */
const buildSession = (user: User): UserSession => ({
  role: user.role,
  lanid: user.login,
  firstName: user.login,
  lastName: '',
  email: 'localuser@localhost',
  isAuth: true,
  exp: 0,
})

export default {
  buildSession,
  roleLevels,
  isAuth,
  isRole,
  hasRole,
  verifyLocalCreds,
}
