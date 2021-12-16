import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('iocs', (table) => {
    table
      .uuid('id')
      .notNullable()
      .unique()
      .primary()
      .comment('Primary key (uuid)')
    table.string('type').notNullable().comment('IOC type')
    table.text('value').comment('IOC value')
    table.boolean('enabled').defaultTo(true).comment('IOC is active')
    table.timestamp('created_at')
    table.unique(['type', 'value'])
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('iocs')
}
