<template>
  <v-container id="source-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>Source Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col col="12" md="4">
                  <v-text-field
                    label="Name"
                    tabindex="1"
                    v-model="name"
                    required
                  >
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="12">
                  <template>
                    <label>Source</label>
                    <prism-editor
                      class="source-editor"
                      v-model="value"
                      :highlight="highlighter"
                      :readonly="loading"
                      line-numbers
                    ></prism-editor>
                  </template>
                </v-col>
              </v-row>
              <v-row> </v-row>
              <v-col col="5" md="2">
                <v-select
                  v-model="secretSelect"
                  :items="secrets"
                  item-text="name"
                  item-value="id"
                  multiple
                  label="Secret"
                ></v-select>
              </v-col>
              <v-row>
                <v-col col="12" md="1">
                  <v-btn color="primary" :disabled="loading" @click="create">
                    Save
                  </v-btn>
                </v-col>
                <v-col md="1">
                  <v-btn
                    color="secondary"
                    @click="scheduleTest"
                    :disabled="loading"
                  >
                    Test
                  </v-btn>
                </v-col>
              </v-row>
            </v-container>
          </v-form>
        </v-card>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <div style="max-height: 50vh; overflow: auto" ref="scanLogs">
          <v-list two-line>
            <template v-for="(entry, key) in entries">
              <v-divider :key="key"></v-divider>
              <v-list-item :key="entry.id">
                <v-list-item-avatar>
                  <v-icon color="blue darken-2">
                    mdi-{{ entryIcon(entry.entry) }}
                  </v-icon>
                </v-list-item-avatar>
                <v-list-item-content>
                  <v-list-item-title v-if="entry.entry === 'screenshot'">
                    <img :src="dataImage(entry.event.payload)" />
                  </v-list-item-title>
                  <v-list-item-title
                    v-else
                    v-html="entry.event.message"
                  ></v-list-item-title>
                </v-list-item-content>
              </v-list-item>
            </template>
            <v-list-item ref="bottom"></v-list-item>
          </v-list>
        </div>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue from 'vue'
import { PrismEditor } from 'vue-prism-editor'
import SourceAPIService, { SecretSelect } from '../../services/sources'
import SecretAPIService from '@/services/secrets'
import ScanLogAPIService, { ScanLogAttributes } from '../../services/scan_logs'

import NotifyMixin from '../../mixins/notify'

import 'vue-prism-editor/dist/prismeditor.min.css'

import { highlight, languages } from 'prismjs/components/prism-core'
import 'prismjs/components/prism-clike'
import 'prismjs/components/prism-javascript'
import 'prismjs/themes/prism-tomorrow.css'

type LogEntryTypes = 'log-message' | 'screenshot' | 'complete' | 'error'

const logTypeIcons: Record<LogEntryTypes, string> = {
  'log-message': 'message',
  screenshot: 'monitor',
  complete: 'check',
  error: 'alert',
}

export default Vue.extend({
  name: 'SourceForm',
  mixins: [NotifyMixin],
  data() {
    return {
      secretSelect: [] as string[],
      entries: [] as ScanLogAttributes[],
      value: 'console.log("code")',
      scanLogTimer: 0,
      name: '',
      loading: false,
      lastLogDate: new Date(),
      testScanID: '',
      secrets: [] as SecretSelect[],
    }
  },
  components: {
    PrismEditor,
  },
  methods: {
    highlighter(code: string) {
      return highlight(code, languages.js)
    },
    dataImage(data: string) {
      return `data:image/jpeg;base64, ${data}`
    },
    entryIcon(entry: LogEntryTypes) {
      return logTypeIcons[entry] ? logTypeIcons[entry] : 'alert-box'
    },
    async create() {
      this.loading = true
      try {
        await SourceAPIService.create({
          source: {
            name: this.name,
            value: this.value,
            secret_ids: this.secretSelect,
          },
        })
        this.$router.push('/sources')
        // redirect to source index on create
      } catch (e) {
        this.errorHandler(e)
      } finally {
        this.loading = false
      }
    },
    async scheduleTest() {
      this.loading = true
      try {
        const res = await SourceAPIService.test({
          source: {
            name: this.name,
            value: this.value,
            secret_ids: this.secretSelect,
          },
        })
        this.entries = []
        this.testScanID = res.data.scan_id
        this.lastLogDate = new Date()
        this.scanLogTimer = setInterval(this.getScanLogs, 3000)
      } catch (e) {
        this.errorHandler(e)
        this.loading = false
      }
    },
    async getScanLogs() {
      const res = await ScanLogAPIService.list({
        scan_id: this.testScanID,
        from: this.lastLogDate,
        pageSize: 128,
        entry: ['log-message', 'screenshot', 'complete', 'error', 'rule-alert'],
        orderColumn: 'created_at',
        orderDirection: 'asc',
      })
      if (res.data.total > 0) {
        this.entries.push(...res.data.results)
        this.lastLogDate = new Date(
          res.data.results[res.data.results.length - 1].created_at
        )
        res.data.results.forEach((r) => {
          if (r.entry === 'complete' || r.entry === 'error') {
            clearInterval(this.scanLogTimer)
            this.loading = false
          }
        })
        this.$nextTick(() => {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          const cl = (this.$refs.bottom as any).$el
          cl.scrollIntoView({ behavior: 'smooth' })
        })
      }
    },
  },
  beforeDestroy() {
    if (this.scanLogTimer) {
      clearInterval(this.scanLogTimer)
    }
  },
  async created() {
    // Copying from existing source / lookup values
    if (this.$route.query.id && typeof this.$route.query.id === 'string') {
      await SourceAPIService.view({
        id: this.$route.query.id,
        eager: ['secrets'],
      })
        .then((res) => {
          this.name = res.data.name
          this.value = res.data.value
          // map optional secrets
          if (res.data.secrets && Array.isArray(res.data.secrets)) {
            const ids = res.data.secrets.map((s) => s.id) as string[]
            if (Array.isArray(ids)) {
              this.secretSelect = ids
            }
          }
        })
        .catch(this.errorHandler)
    }

    await SecretAPIService.list({
      fields: ['id', 'name', 'type'],
      pageSize: 200,
    }).then((res) => {
      this.secrets = res.data.results
    })
  },
})
</script>

<style scoped>
/* required class */
.source-editor {
  /* we dont use `language-` classes anymore so thats why we need to add background and text color manually */
  background: #2d2d2d;
  color: #ccc;
  height: 300px;
  /* you must provide font-family font-size line-height. Example: */
  font-family: Fira code, Fira Mono, Consolas, Menlo, Courier, monospace;
  font-size: 14px;
  line-height: 1.5;
  padding: 5px;
}

/* optional class for removing the outline */
.prism-editor__textarea:focus {
  outline: none;
}
</style>
