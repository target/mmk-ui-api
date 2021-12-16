import Vue from 'vue'

export interface TableMixinBindings {
  page: number
  pageCount: number
  total: number
  itemsPerPage: number
  loading: boolean
  sortBy: string[]
  sortDesc: boolean[]
  resolveOrder(): () => { orderColumn?: string; orderDirection?: string }
}

export default Vue.extend({
  data() {
    return {
      page: 1,
      pageCount: 0,
      total: 0,
      itemsPerPage: 10,
      loading: true,
      sortBy: ['created_at'],
      sortDesc: ['desc'],
    }
  },
  methods: {
    /**
     * resolves the sort order by column name and direction
     *
     * Returns an empty object if no sort options are set
     */
    resolveOrder() {
      const ret: { orderColumn?: string; orderDirection?: string } = {}
      if (this.sortBy.length) {
        ret.orderColumn = this.sortBy[0]
        if (this.sortDesc.length) {
          ret.orderDirection = this.sortDesc[0] ? 'desc' : 'asc'
        }
      }
      return ret
    },
  },
})
