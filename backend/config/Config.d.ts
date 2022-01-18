/* tslint:disable */
/* eslint-disable */
declare module "node-config-ts" {
  interface IConfig {
    port: number
    env: string
    server: Server
    auth: Auth
    redis: Redis
    postgres: Postgres
    session: Session
    oauth: Oauth
    transport: Transport
    authorizations: Authorizations
    quantumTunnel: QuantumTunnel
    alerts: Alerts
  }
  interface Alerts {
    goAlert: GoAlert
    kafka: Kafka
  }
  interface Kafka {
    enabled: boolean
    host: string
    port: number
    topic: string
    cert: string
    key: string
    clientID: string
  }
  interface GoAlert {
    enabled: boolean
    url: string
    token: undefined
  }
  interface QuantumTunnel {
    enabled: string
    clientID: string
    secret: string
    user: string
    password: string
    key: string
    url: string
  }
  interface Authorizations {
    admin: string
    user: string
    transport: string
  }
  interface Transport {
    cert: undefined
    key: undefined
    port: number
    mTLS: boolean
    enabled: boolean
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
  interface Postgres {
    host: string
    user: string
    password: string
    database: string
    secure: boolean
    ca: string
  }
  interface Redis {
    uri: string
    useSentinel: boolean
    nodes: string[]
    master: string
    sentinelPort: number
    sentinelPassword: string
  }
  interface Auth {
    strategy: string
  }
  interface Server {
    uri: string
    ca: string
  }
  export const config: Config
  export type Config = IConfig
}
