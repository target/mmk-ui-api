<template>
  <v-row>
    <v-col cols="12" md="4">
      <v-card class="mx-auto" outlined>
        <v-card-title class="secondary ligthen-1 white--text">
          Domains
        </v-card-title>
        <v-card-text>
          <template v-if="summary.totalReq > 0">
            <v-list dense class="scroll" height="300px">
              <v-list-item
                v-for="[domain, total] in summary.requests.domain"
                :key="domain"
              >
              <v-list-item-icon>
                <v-icon>mdi-web</v-icon>
              </v-list-item-icon>
              <v-list-item-content>
                <v-list-item-title>
                  {{ domain }}
                </v-list-item-title>
              </v-list-item-content>
                {{ total.toLocaleString() }}
              </v-list-item>
            </v-list>
          </template>
          <template>
            <h3 class="text-center">
              None
            </h3>
          </template>
        </v-card-text>
      </v-card>
    </v-col>
    <v-col cols="12" md="3">
      <v-card class="mx-auto" outlined>
        <v-card-title class="secondary ligthen-1 white--text">
          Totals
        </v-card-title>
        <v-card-text>
          <v-list dense height="300px">
            <v-list-item v-for="count in counts" :key="count.key">
              <v-list-item-icon>
                <v-icon>{{ count.icon }}</v-icon>
              </v-list-item-icon>
              <v-list-item-content>
                <v-list-item-title>
                  {{ count.title }}
                </v-list-item-title>
              </v-list-item-content>
              {{ summary[count.key].toLocaleString() }}
            </v-list-item>
          </v-list>
        </v-card-text>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts">
import Vue from 'vue'
import ScanAPIService from '@/services/scans'

import NotifyMixin from '../../mixins/notify'

const summaryTotals = [
  {
    icon: 'mdi-lan-connect',
    title: 'Requests',
    key: 'totalReq'
  },
  {
    icon: 'mdi-alert',
    title: 'Alerts',
    key: 'totalAlerts'
  },
  {
    icon: 'mdi-skull',
    title: 'Errors',
    key: 'totalErrors'
  },
  {
    icon: 'mdi-code-parentheses',
    title: 'Function Calls',
    key: 'totalFunc'
  },
  {
    icon: 'mdi-cookie',
    title: 'Cookies',
    key: 'totalCookies'
  }
]

export default Vue.extend({
  name: 'ScanSummary',
  props: {
    scanID: String
  },
  mixins: [NotifyMixin],
  data() {
    return {
      init: false,
      scan: {},
      summary: {},
      counts: [{}]
    }
  },
  methods: {
    async getScan() {
      const res = await ScanAPIService.view({
        id: this.scanID,
        eager: ['sites']
      })
      this.scan = res.data
    },
    async getSummary() {
      const res = await ScanAPIService.summary({ id: this.scanID })
      this.summary = res.data
    }
  },
  async created() {
    try {
      await this.getScan()
      await this.getSummary()
      this.counts = summaryTotals
      Object.freeze(this.counts)
      this.init = true
    } catch (e) {
      this.errorHandler(e)
    }
  }
})
</script>

<style scoped>
.scroll {
  overflow-y: scroll;
}
</style>
