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
  ['ioc_payload_wallet_clear_cc', 'https://example.com?cc=5424180279791765', 1],
  [
    'ioc_payload_wallet_b64_cc',
    'https://example.com?cc=NTQyNDE4MDI3OTc5MTc2NQ',
    1,
  ],
  [
    'ioc_payload_giftcards_clear_cc',
    'https://example.com?cc=041399012207400',
    1,
  ],
  [
    'ioc_payload_giftcards_clear_cc',
    'https://example.com?cc=439408432908905',
    1,
  ],
  [
    'ioc_payload_giftcards_b64_cc',
    'https://example.com?cc=MDQxMzk5MDEyMjA3NDAw',
    1,
  ],
  [
    'ioc_payload_giftcards_b64_cc',
    'https://example.com?cc=NDM5NDA4NDMyOTA4OTA1',
    1,
  ],
  ['ioc_payload_guest_clear_cc', 'https://example.com?cc=222334454657563', 1],
  [
    'ioc_payload_guest_base64_cc',
    'https://example.com?cc=MjIyMzM0NDU0NjU3NTYz',
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
