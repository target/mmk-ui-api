<template>
  <v-container id="scan-log" fluid tag="section" v-if="init">
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
        >
          <template v-slot:top>
            <v-toolbar flat id="scan-log-toolbar">
              <v-toolbar-title
                >{{ scan.site ? scan.site.name : 'No Site' }} /
                {{
                  scan.source ? scan.source.name : 'No Source'
                }}</v-toolbar-title
              >
              <v-spacer></v-spacer>
              <v-text-field
                color="secondary"
                hide-details
                label="Search"
                v-model="search"
                @keyup.enter="runSearch"
              >
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
                  v-model="entryFilter"
                  :items="entryTypes"
                  multiple
                  chips
                  label="Entries"
                >
                </v-select>
              </v-toolbar-items>
            </v-toolbar>
          </template>

          <template v-slot:[`item.event`]="{ item }">
            <span v-if="item.event !== null">
              <span v-if="item.entry === 'screenshot'" class="entry-screenshot">
                <img :src="dataImage(item.event.payload)" />
              </span>
              <vue-json-pretty
                v-else
                class="pretty-wrap"
                :data="item.event"
                :customValueFormatter="escapeHTML"
              >
              </vue-json-pretty>
            </span>
            <span v-else-if="typeof item.event === 'string'">{{
              item.event
            }}</span>
            <span v-else>general</span>
          </template>
        </v-data-table>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'
import VueJsonPretty from 'vue-json-pretty'
import 'vue-json-pretty/lib/styles.css'
import ScanLogAPIService, { ScanLogAttributes } from '@/services/scan_logs'
import ScanAPIService, { ScanAttributes } from '@/services/scans'
import '../../assets/sass/scan-logs.scss'

import TableMixin, { TableMixinBindings } from '@/mixins/table'

type LogEntryTypes =
  | 'log-message'
  | 'screenshot'
  | 'complete'
  | 'error'
  | 'failed'

const escapeEntit: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  "'": '&#39;',
  '"': '&quot;',
}

const logTypeIcons: Record<LogEntryTypes, string> = {
  'log-message': 'message',
  screenshot: 'monitor',
  complete: 'check',
  error: 'alert',
  failed: 'alert',
}

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  name: 'ScanLog',
  mixins: [TableMixin],
  data() {
    return {
      records: [] as ScanLogAttributes[],
      entryTypes: [] as string[],
      scanID: this.$route.params.id as string,
      search: '',
      init: false,
      options: {},
      entryFilter: [] as string[],
      headers: Object.freeze([
        {
          text: 'Date',
          align: 'start',
          sortable: true,
          value: 'created_at',
          width: '200px',
        },
        {
          text: 'Entry',
          width: '120px',
          value: 'entry',
        },
        {
          text: 'Event',
          width: 500,
          sortable: false,
          value: 'event',
        },
        {
          text: 'Level',
          width: '120px',
          value: 'level',
        },
      ]),
      scan: {} as ScanAttributes,
    }
  },
  watch: {
    options: {
      handler() {
        this.$nextTick(() => {
          this.getLogs()
        })
      },
      deep: true,
    },
    entryFilter() {
      this.$nextTick(() => {
        this.getLogs()
      })
    },
  },
  methods: {
    escapeHTML(data: unknown): unknown {
      if (typeof data !== 'string') return data
      const ret = data.replace(/[&<>'"]/g, (tag: string) => {
        if (tag in escapeEntit) {
          return escapeEntit[tag]
        }
        return ''
      })
      return `"${ret}"`
    },
    runSearch(): void {
      this.page = 1
      this.getLogs()
    },
    getDistinct(): void {
      ScanLogAPIService.distinct({ column: 'entry', id: this.scanID })
        .then((res) => {
          this.entryTypes = res.data.map((e) => e.entry)
        })
        .catch(console.error)
    },
    getScan(): void {
      ScanAPIService.view({
        id: this.scanID,
        eager: ['sources', 'sites'],
      })
        .then((res) => {
          this.init = true
          this.scan = res.data
        })
        .catch(console.error)
    },
    getLogs(): void {
      ScanLogAPIService.list({
        fields: ['event', 'entry', 'level', 'created_at'],
        scan_id: this.scanID,
        page: this.page,
        entry: this.entryFilter,
        pageSize: this.itemsPerPage,
        search: this.search,
        ...this.resolveOrder(),
      })
        .then((res) => {
          this.records = res.data.results
          this.total = res.data.total
        })
        .catch((err) => {
          console.error(err)
        })
        .finally(() => (this.loading = false))
    },
    dataImage: (data: string) => `data:image/jpeg;base64, ${data}`,
    entryIcon: (entry: LogEntryTypes) =>
      logTypeIcons[entry] ? logTypeIcons[entry] : 'alert-box',
  },
  created() {
    this.scanID = this.$route.params.id
    this.getDistinct()
    this.getScan()
  },
  components: {
    VueJsonPretty,
  },
})
</script>

<style>
.vjs-tree.is-root {
  position: revert;
}
.pretty-wrap {
  max-width: 60vw;
}
.vjs-tree .vjs-value__string {
  word-break: break-all;
}
</style>
