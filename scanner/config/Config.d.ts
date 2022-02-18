/* tslint:disable */
/* eslint-disable */
declare module "node-config-ts" {
  interface IConfig {
    port: undefined
    env: undefined
    redis: Redis
    session: Session
    oauth: Oauth
    transport: Transport
  }
  interface Transport {
    http: undefined
  }
  interface Oauth {
    authURL: undefined
    tokenURL: undefined
    clientID: undefined
    secret: undefined
    redirectURL: undefined
    scope: undefined
  }
  interface Session {
    secret: string
    maxAge: number
  }
  interface Redis {
    uri: undefined
    useSentinel: undefined
    nodes: undefined
    master: undefined
    sentinelPort: undefined
    sentinelPassword: undefined
  }
  export const config: Config
  export type Config = IConfig
}
