import Ajv from 'ajv'

import logger from '../loaders/logger'

const ajv = new Ajv()

/**
 * isOfType
 *
 * Run-time type guard using AJV for schema validation
 */
export const isOfType = <T>(
  value: unknown,
  schema: string | boolean | Record<string, unknown>
): value is T => {
  ajv.validate(schema, value)
  if (ajv.errors === null) {
    return true
  }
  logger.error({
    component: 'lib/utils#isOfType',
    message: 'Failed Validation',
    context: {
      value,
      schema: JSON.stringify(schema),
    },
    errors: ajv.errorsText()
  })
  throw new Error(`run-time type guard ${ajv.errorsText()}`)
}
