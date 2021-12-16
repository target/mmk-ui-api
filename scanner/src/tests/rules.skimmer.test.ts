import path from 'path'
import fs from 'fs'
import YaraSync from '../lib/yara-sync'
import { js } from 'js-beautify'

const yara = new YaraSync()

const samplesPath = path.resolve(__dirname, 'samples')

const testCases = [
  ['slow-aes.js', 'digital_skimmer_slowaes', 1],
  ['gibberish-aes.js', 'digital_skimmer_obfuscated_gibberish', 1],
  ['gibberish-obf.js', 'digital_skimmer_obfuscated_gibberish', 1],
  ['cryptojs.core.min.js', 'digital_skimmer_cryptojs', 1],
  ['jsencrypt.js', 'digital_skimmer_jsencrypt', 1],
  ['caesar.js', 'digital_skimmer_caesar_obf', 1],
  ['freshchat.js.sample', 'digital_skimmer_freshchat_obf', 1],
  ['give-basic-sample', 'digital_skimmer_giveme_obf', 1],
  // ['obfu-alt.js', 'digital_skimmer_obfuscatorio_obf', 1],
  ['basic.js', 'digital_skimmer_obfuscatorio_obf', 1],
  ['loop-commerce.js', false, 0],
]

describe('Skimmer Rules', () => {
  beforeAll(async () => {
    try {
      await yara.initAsync({
        rules: [
          { filename: path.resolve(__dirname, '../rules', 'skimmer.yara') },
        ],
      })
    } catch (e) {
      console.log('error', e)
      throw e
    }
  })

  test.each(testCases)(
    'detects %p as %p',
    async (sampleFile, expectedID, expectedMatches) => {
      const buffer = fs.readFileSync(`${samplesPath}/${sampleFile}`).toString()
      const result = await yara.scanAsync({
        buffer: Buffer.from(js(buffer), 'utf-8'),
      })
      if (expectedID) {
        expect(result.rules[0].id).toBe(expectedID)
      }
      expect(result.rules.length).toBe(expectedMatches)
    }
  )
})
