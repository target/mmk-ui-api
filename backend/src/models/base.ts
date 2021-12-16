import { Model, ModelClass } from 'objection'

export default abstract class BaseModel<M> extends Model {
  updateAble(): Array<keyof M> | [] {
    return []
  }
  insertAble(): Array<keyof M> | [] {
    return []
  }
  selectAble(): Array<keyof M> | [] {
    return []
  }
}

export interface BaseClass<M extends Model> extends Partial<ModelClass<M>> {
  updateAble?(): Array<keyof M>
  insertAble?(): Array<keyof M>
  selectAble?(): Array<keyof M>
  new (): M
}
