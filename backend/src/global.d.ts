interface UserSession {
  lanid?: string
  nonce?: string
  firstName?: string
  lastName?: string
  role?: UserRole
  email?: string
  isAuth?: boolean
  exp?: number
}

declare module 'yara'
