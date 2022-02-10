<template>
  <v-container id="alerts" fluid tag="section">
    <v-row>
      <v-col cols="12">
        <v-data-table
          :headers="headers"
          :items="records"
          :options.sync="options"
          :server-items-length="total"
          :page.sync="page"
          :sort-by.sync="sortBy"
          :sort-desc.sync="sortDesc"
          :loading="loading"
          :items-per-page.sync="itemsPerPage"
          :footer-props="{ itemsPerPageOptions: [10, 25, 50, 100, -1] }"
          class="elevation-1"
          @page-count="pageCount = $event"
          :expanded.sync="expanded"
          show-expand
        >
          <template v-slot:top>
            <v-toolbar flat>
              <v-toolbar-title>Alerts</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-text-field color="secondary" hide-details label="Search" 
                v-model="search" @keyup.enter="runSearch">
                <template v-slot:append-outer>
                  <v-btn
                    class="mt-n2"
                    elevation="1"
                    fab
                    small
                    @click="runSearch"
                  >
                    <v-icon>mdi-magnify</v-icon>
                  </v-btn>
                </template>
              </v-text-field>
              <v-spacer></v-spacer>
              <v-toolbar-items>
                <v-select
                  v-model="ruleFilter"
                  :items="ruleTypes"
                  multiple
                  chips
                  label="Rules"
                >
                </v-select>
              </v-toolbar-items>
            </v-toolbar>
          </template>

          <template v-slot:[`item.site.name`]="{ item }">
            <span v-if="item.scan_id">
              <router-link
                :to="{ name: 'ScanLog', params: { id: item.scan_id } }"
                style="text-decoration: none; color: inherit"
                title="View Scan"
              >
                {{ item.site.name }} <v-icon>mdi-magnify-expand</v-icon>
              </router-link>
            </span>
            <span v-else>
              {{ item.site.name }}
            </span>
          </template>

          <template v-slot:expanded-item="{ headers, item }">
            <td class="context pa-md-4" :colspan="headers.length + 1">
              <span v-if="item.context !== null">
                <vue-json-pretty class="pretty-wrap" :data="item.context">
                </vue-json-pretty>
              </span>
            </td>
          </template>
        </v-data-table>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'

import AlertAPIService, { AlertAttributes } from '@/services/alerts'
import VueJsonPretty from 'vue-json-pretty'
import 'vue-json-pretty/lib/styles.css'

import TableMixin, { TableMixinBindings } from '@/mixins/table'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin],
  data() {
    return {
      options: {},
      ruleTypes: [] as string[],
      ruleFilter: [] as string[],
      search: '',
      expanded: [],
      headers: Object.freeze([
        {
          text: '',
          value: 'data-table-expand',
        },
        {
          text: 'Rule',
          align: 'start',
          sortable: true,
          value: 'rule',
        },
        {
          text: 'message',
          sortable: false,
          value: 'message',
        },
        {
          text: 'Site',
          sortable: false,
          value: 'site.name',
        },
        {
          text: 'Created',
          value: 'created_at',
        },
      ]),
      records: [] as AlertAttributes[],
    }
  },
  watch: {
    options: {
      handler() {
        this.$nextTick(() => {
          this.list()
        })
      },
      deep: true,
    },
    ruleFilter() {
      this.$nextTick(() => {
        this.list()
      })
    },
  },
  methods: {
    async list() {
      const res = await AlertAPIService.list({
        fields: ['id', 'rule', 'message', 'created_at', 'scan_id', 'context'],
        eager: ['site'],
        page: this.page,
        pageSize: this.itemsPerPage,
        rule: this.ruleFilter,
        search: this.search,
        ...this.resolveOrder(),
      })
      res.data.results.forEach((res) => {
        if (res.site === null) {
          res.site = { name: 'Deleted' }
        }
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    getDistinct() {
      AlertAPIService.distinct({ column: 'rule' })
        .then((res) => {
          this.ruleTypes = res.data.map((e) => e.rule)
        })
        .catch(this.errorHandler)
    },
    runSearch(): void {
      this.page = 1
      this.list()
    },
  },
  created() {
    this.getDistinct()
  },
  components: {
    VueJsonPretty,
  },
})
</script>
