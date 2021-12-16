import fs from 'fs'
import { ConnectionOptions } from 'tls'
import { config } from 'node-config-ts'
import { Knex } from 'knex'

const connOptions: Knex.PgConnectionConfig = {
  host: config.postgres.host,
  user: config.postgres.user,
  database: config.postgres.database,
  password: config.postgres.password
}

if (config.postgres.secure) {
  connOptions.ssl = {
    rejectUnauthorized: false,
    ca: fs.readFileSync(config.postgres.ca).toString()
  } as ConnectionOptions
}

const knexConfig = {
  client: 'pg',
  useNullAsDefault: true,
  connection: connOptions,
  pool: {
    min: 2,
    max: 10
  },
  migrations: {
    tableName: 'knex_migrations',
    directory: './src/migrations',
    disableTransactions: true
  }
}

module.exports = {
  test: knexConfig,
  development: knexConfig,
  staging: knexConfig,
  production: knexConfig
}
