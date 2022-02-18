/* tslint:disable */
/* eslint-disable */
declare module "node-config-ts" {
  interface IConfig {
    port: undefined
    env: string
    redis: Redis
    session: Session
    oauth: Oauth
    transport: Transport
  }
  interface Transport {
    http: string
  }
  interface Oauth {
    authURL: string
    tokenURL: string
    clientID: string
    secret: string
    redirectURL: string
    scope: string
  }
  interface Session {
    secret: string
    maxAge: number
  }
  interface Redis {
    uri: string
    useSentinel: boolean
    nodes: string[]
    master: string
    sentinelPort: number
    sentinelPassword: string
  }
  export const config: Config
  export type Config = IConfig
}
