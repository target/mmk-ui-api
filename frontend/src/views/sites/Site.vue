<template type="html">
  <v-container id="site" fluid tag="section" v-if="site.name">
    <v-row>
      <v-col cols="12">
        <v-card>
          <v-toolbar dark flat>
            <v-toolbar-title>{{ site.name }}</v-toolbar-title>
            <template v-slot:extension>
              <v-tabs v-model="tab" align-with-title dark>
                <v-tab key="alerts"> Alerts </v-tab>
                <v-tab key="scans"> Scans </v-tab>
              </v-tabs>
            </template>
          </v-toolbar>
          <v-tabs-items v-model="tab">
            <v-tab-item
              :transition="false"
              :reverse-transition="false"
              key="alerts"
            >
              <v-data-table
                :headers="alert.headers"
                :items="alertRecords"
                :options.sync="alertOptions"
                :server-items-length="alert.total"
                :page.sync="alert.page"
                :sort-by.sync="alert.sortBy"
                :sort-desc.sync="alert.sortDesc"
                :loading="alert.loading"
                :items-per-page.sync="alert.itemsPerPage"
                :footer-props="{ itemsPerPageOptions: [10, 25, 50, 100, -1] }"
                class="elevation-1"
                @page-count="alert.pageCount = $event"
              >
                <template v-slot:top>
                  <v-toolbar flat>
                    <v-col col="12" md="3">
                      <v-text-field
                        color="secondary"
                        hide-details
                        label="Search"
                      >
                        <template v-slot:append-outer>
                          <v-btn class="mt-n2" elevation="1" fab small>
                            <v-icon>mdi-magnify</v-icon>
                          </v-btn>
                        </template>
                      </v-text-field>
                    </v-col>
                  </v-toolbar>
                </template>

                <template v-slot:[`item.scan_id`]="{ item }">
                  <span v-if="item.scan_id">
                    <router-link
                      :to="{ name: 'ScanLog', params: { id: item.scan_id } }"
                      style="text-decoration: none; color: inherit"
                    >
                      Scan
                    </router-link>
                  </span>
                  <span v-else> Deleted </span>
                </template>
              </v-data-table>
            </v-tab-item>
            <v-tab-item
              :transition="false"
              :reverse-transition="false"
              key="scans"
            >
              <v-data-table
                :headers="scan.headers"
                :items="scanRecords"
                :options.sync="scanOptions"
                :server-items-length="scan.total"
                :page.sync="scan.page"
                :sort-by.sync="scan.sortBy"
                :sort-desc.sync="scan.sortDesc"
                :loading="scan.loading"
                :items-per-page.sync="scan.itemsPerPage"
                :footer-props="{ itemsPerPageOptions: [10, 25, 50, 100, -1] }"
                class="elevation-1"
                @page-count="scan.pageCount = $event"
              >
                <template v-slot:[`item.created_at`]="{ item }">
                  <router-link
                    :to="{ name: 'ScanLog', params: { id: item.id } }"
                    style="text-decoration: none; color: inherit"
                  >
                    {{ item.created_at }}
                  </router-link>
                </template>
              </v-data-table>
            </v-tab-item>
          </v-tabs-items>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import SiteAPIService, { SiteAttributes } from '@/services/sites'
import ScanAPIService, { ScanAttributes } from '@/services/scans'
import AlertAPIService, { AlertAttributes } from '@/services/alerts'
import Vue from 'vue'

import '../../assets/sass/scan-logs.scss'

export default Vue.extend({
  name: 'SiteView',
  data() {
    return {
      tab: null,
      site: {} as SiteAttributes,
      alertOptions: {},
      loading: true,
      alertRecords: [] as AlertAttributes[],
      alert: {
        page: 1,
        pageCount: 0,
        total: 0,
        sortBy: [],
        sortDesc: false,
        itemsPerPage: 10,
        loading: true,
        headers: Object.freeze([
          {
            text: 'Date',
            align: 'start',
            sortable: true,
            value: 'created_at',
            width: '220px',
          },
          {
            text: 'Rule',
            value: 'rule',
            width: '300px',
          },
          {
            text: 'Message',
            value: 'message',
            sortable: false,
          },
          {
            text: 'Scan',
            value: 'scan_id',
            sortable: false,
          },
        ]),
      },
      scanOptions: {},
      scanRecords: [] as ScanAttributes[],
      scan: {
        page: 1,
        pageCount: 0,
        total: 0,
        sortBy: [],
        sortDesc: false,
        itemsPerPage: 10,
        loading: true,
        headers: Object.freeze([
          {
            text: 'Date',
            align: 'start',
            sortable: true,
            value: 'created_at',
            width: '220px',
          },
          {
            text: 'State',
            value: 'state',
          },
        ]),
      },
    }
  },
  watch: {
    alertOptions: {
      handler() {
        this.$nextTick(() => {
          this.getAlerts()
        })
      },
      deep: true,
    },
    scanOptions: {
      handler() {
        this.$nextTick(() => {
          this.getScans()
        })
      },
    },
  },
  methods: {
    async getSite() {
      this.loading = true
      const res = await SiteAPIService.view({ id: this.$route.params.id })
      this.site = res.data
    },
    async getAlerts() {
      this.alert.loading = true
      const res = await AlertAPIService.list({
        site_id: this.$route.params.id,
        page: this.alert.page,
        pageSize: this.alert.itemsPerPage,
        orderColumn: this.alert.sortBy[0],
        orderDirection: this.alert.sortDesc ? 'desc' : 'asc',
      })
      this.alert.loading = false
      this.alertRecords = res.data.results
      this.alert.total = res.data.total
    },
    async getScans() {
      this.scan.loading = true
      const res = await ScanAPIService.list({
        site_id: this.$route.params.id,
        page: this.scan.page,
        pageSize: this.scan.itemsPerPage,
        orderColumn: this.scan.sortBy[0],
        orderDirection: this.scan.sortDesc ? 'desc' : 'asc',
      })
      this.scan.loading = false
      this.scanRecords = res.data.results
      this.scan.total = res.data.total
    },
  },
  async created() {
    await this.getSite()
  },
})
</script>
