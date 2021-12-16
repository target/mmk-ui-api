import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('files', (table) => {
    table.uuid('id').notNullable().primary().comment('Primary key (uuid)')
    table.string('url').notNullable().comment('Source URL')
    table.string('filename').notNullable().comment('Resolved filename')
    table.string('sha256').notNullable().unique().comment('File SHA256 digest')
    table.json('headers')
    table.timestamp('created_at')
    table
      .uuid('scan_id')
      .references('id')
      .inTable('scans')
      .notNullable()
      .onDelete('CASCADE')
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('files')
}
