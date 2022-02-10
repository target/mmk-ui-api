import { stripJSONUnicode } from '../lib/utils'

describe('stripJSONUnicode', () => {
  it('should strip non-ascii characters from POJOs', () => {
    expect(stripJSONUnicode({ event: 'Öfoo' })).toEqual({ event: 'foo' })
  })
  it('should strip null characters from POJOs', () => {
    expect(stripJSONUnicode({ event: 'foo\u0000' })).toEqual({ event: 'foo' })
  })
})
