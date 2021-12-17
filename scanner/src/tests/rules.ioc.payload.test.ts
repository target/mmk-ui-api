import path from 'path'
import YaraSync from '../lib/yara-sync'

const yara = new YaraSync()

const testCases = [
  [
    'ioc_payload_checkout_clear_cc',
    'https://example.com?cc=4012000077777777',
    1,
  ],
  [
    'ioc_payload_checkout_b64_cc',
    'https://example.com?cc=NDAxMjAwMDA3Nzc3Nzc3Nw',
    1,
  ],
]

describe('IOC Payload Rules', () => {
  beforeAll(async () => {
    try {
      await yara.initAsync({
        rules: [
          {
            filename: path.resolve(__dirname, '../rules', 'ioc.payloads.yara'),
          },
        ],
      })
    } catch (e) {
      console.log('error', e)
      throw e
    }
  })

  test.each(testCases)(
    'detects %p as %p',
    async (expectedID, sample, expectedMatches) => {
      const result = await yara.scanAsync({
        buffer: Buffer.from(sample as string, 'utf-8'),
      })
      if (expectedID) {
        expect(result.rules[0].id).toBe(expectedID)
      }
      expect(result.rules.length).toBe(expectedMatches)
    }
  )
})
