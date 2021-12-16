import Chance from 'chance'
import ModelFactory from './base'
import { Source, SourceAttributes } from '../../models'

const chance = new Chance()

export default new ModelFactory<Source, SourceAttributes>(
  {
    value: 'console.log("value")',
    get name() {
      return chance.name()
    },
  },
  Source
)
