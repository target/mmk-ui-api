import fs from 'fs'
import _knex, { Knex } from 'knex'

import { config } from 'node-config-ts'
import Alert, { AlertAttributes } from './alerts'
import AllowList, { AllowListAttributes } from './allow_list'
import File, { FileAttributes } from './files'
import Site, { SiteAttributes } from './sites'
import Ioc, { IocAttributes } from './iocs'
import SeenString, { SeenStringAttributes } from './seen_strings'
import Source, { SourceAttributes } from './sources'
import SourceSecret, { SourceSecretAttributes } from './source_secrets'
import Scan, { ScanAttributes } from './scans'
import ScanLog, { ScanLogAttributes } from './scan_logs'
import Secret, { SecretAttributes } from './secrets'
import User, { UserAttributes } from './users'
import logger from '../loaders/logger'

const connOptions: Knex.PgConnectionConfig = {
  host: config.postgres.host,
  user: config.postgres.user,
  database: config.postgres.database,
  password: config.postgres.password,
}

if (config.postgres.secure) {
  connOptions.ssl = {
    rejectUnauthorized: false,
    ca: fs.readFileSync(config.postgres.ca).toString(),
  }
}

const knex = _knex({
  client: 'postgresql',
  connection: connOptions,
  log: {
    warn(message) {
      logger.silent(message)
    },
    error: logger.error,
    deprecate: logger.silent,
    debug: logger.silent,
  },
})

Site.knex(knex)
Ioc.knex(knex)
SeenString.knex(knex)
Source.knex(knex)
Scan.knex(knex)
Secret.knex(knex)
SourceSecret.knex(knex)
Alert.knex(knex)
AllowList.knex(knex)
File.knex(knex)
ScanLog.knex(knex)
User.knex(knex)

export {
  Alert,
  AlertAttributes,
  AllowList,
  AllowListAttributes,
  File,
  FileAttributes,
  Site,
  SiteAttributes,
  Ioc,
  IocAttributes,
  SeenString,
  SeenStringAttributes,
  Scan,
  ScanAttributes,
  ScanLog,
  ScanLogAttributes,
  Source,
  SourceAttributes,
  SourceSecret,
  SourceSecretAttributes,
  Secret,
  SecretAttributes,
  User,
  UserAttributes,
  knex,
}
