import axios from 'axios'

export interface Queues {
  schedule: number
  event: number
  scanner: number
}

const view = async () => axios.get<Queues>('/api/queues')

export default {
  view,
}
