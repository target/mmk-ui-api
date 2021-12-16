import { Model } from 'objection'
import { BaseClass } from '../../models/base'

export default class ModelFactory<T extends Model, M> {
  constructor(protected defaults: Partial<M>, private model: BaseClass<T>) {}

  build(p?: Partial<M>): T {
    return this.model.fromJson({
      ...this.defaults,
      ...p,
    })
  }
}
