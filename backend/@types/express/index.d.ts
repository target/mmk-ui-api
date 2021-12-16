declare global {
  declare module 'express-session' {
    import 'express-session'
    export interface Session {
      data: UserSession
    }
  }
}
