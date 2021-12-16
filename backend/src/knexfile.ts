import fs from 'fs'
import { config } from 'node-config-ts'
import { Knex } from 'knex'

const connOptions: Knex.PgConnectionConfig = {
  host: config.postgres.host,
  user: config.postgres.user,
  database: config.postgres.database,
  password: config.postgres.password,
  ssl: false,
}

if (config.postgres.secure) {
  connOptions.ssl = {
    rejectUnauthorized: false,
    ca: fs.readFileSync(config.postgres.ca).toString(),
  }
}

const knexConfig = {
  client: 'pg',
  useNullAsDefault: true,
  connection: connOptions,
  pool: {
    min: 2,
    max: 10,
  },
  migrations: {
    tableName: 'knex_migrations',
    directory: './migrations',
  },
}

console.log(knexConfig)

module.exports = {
  test: knexConfig,
  development: knexConfig,
  staging: knexConfig,
  production: knexConfig,
}
