/**
 * stripJSONUnicode
 *
 * Removes unicode and null characters from a JSON object
 */
export const stripJSONUnicode = (obj: unknown) =>
  JSON.parse(JSON.stringify(obj, null).replace(/([^ -~]|\\u0000)+/g, ''))
