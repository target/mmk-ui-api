import { Knex } from 'knex'

export async function up(knex: Knex): Promise<void> {
  return knex.schema.createTable('users', (table) => {
    table
      .uuid('id')
      .notNullable()
      .unique()
      .primary()
      .comment('Primary key (uuid)')
    table.string('login').notNullable().unique().comment('User Login')
    table.text('password_hash').notNullable().comment('User Password Hash')
    table.string('role').notNullable().comment('User role')
    table.timestamps(true, true)
  })
}

export async function down(knex: Knex): Promise<void> {
  return knex.schema.dropTable('users')
}
