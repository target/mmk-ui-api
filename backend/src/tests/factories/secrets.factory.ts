import Chance from 'chance'
import ModelFactory from './base'
import { Secret, SecretAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<Secret, SecretAttributes>(
  {
    get name() {
      return chance.word()
    },
    type: 'manual',
    get value() {
      return chance.word()
    },
  },
  Secret
)
