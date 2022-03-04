<template>
  <v-container id="dashboard" fluid tag="section">
    <v-row>
      <v-col cols="12" md="6">
        <v-card class="mx-auto" outlined>
          <v-img
            class="white--text align-end"
            height="110px"
            src="@/assets/cyber-header.webp"
          >
            <v-card-title> Alerts </v-card-title>
          </v-img>

          <v-card-text>
            <div class="font-weight-bold ml-8 mb-2">Last 5</div>

            <v-timeline align-top dense>
              <v-timeline-item v-for="item of alerts" :key="item.id" small>
                <div>
                  <div class="font-weight-normal">
                    <strong>{{ item.rule }}</strong> @ {{ item.created_at }}
                  </div>
                  <div>{{ item.message }}</div>
                </div>
              </v-timeline-item>
            </v-timeline>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card class="mx-auto" outlined>
          <v-img
            class="white--text align-end"
            height="110px"
            src="@/assets/board-header.webp"
          >
            <v-card-title>Scans</v-card-title>
          </v-img>
          <v-card-text>
            <div class="font-weight-bold ml-8 mb-2">Last 5</div>
            <v-timeline align-top dense>
              <v-timeline-item
                v-for="item of scans"
                :key="item.id"
                :color="colors[item.state]"
                small
              >
                <div>
                  <div class="font-weight-normal">
                    <strong
                      ><router-link
                        :to="{ name: 'ScanLog', params: { id: item.id } }"
                        style="text-decoration: none; color: inherit"
                        >{{ item.site.name }}
                      </router-link></strong
                    >
                    @
                    {{ item.created_at }}
                  </div>
                  <div>
                    {{ item.state }} / <i>{{ item.source.name }} </i>
                  </div>
                </div>
              </v-timeline-item>
            </v-timeline>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" lg="12">
        <v-card class="mx-auto" outlined>
          <v-list-item>
            <v-list-item-content>
              <v-list-item-title class="headline">
                Job Queues
              </v-list-item-title>
            </v-list-item-content>
          </v-list-item>
          <v-container fluid>
            <v-row dense>
              <v-col cols="4">
                <v-card>
                  <v-card-title class="justify-center">{{
                    queues.schedule
                  }}</v-card-title>
                  <v-card-text> Scheduled </v-card-text>
                </v-card>
              </v-col>
              <v-col cols="4">
                <v-card>
                  <v-card-title class="justify-center">{{
                    queues.event
                  }}</v-card-title>
                  <v-card-text> Browser Events </v-card-text>
                </v-card>
              </v-col>
              <v-col cols="4">
                <v-card>
                  <v-card-title class="justify-center">{{
                    queues.scanner
                  }}</v-card-title>
                  <v-card-text> Scan Jobs </v-card-text>
                </v-card>
              </v-col>
            </v-row>
          </v-container>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'
import AlertAPIService, { AlertAttributes } from '@/services/alerts'
import ScanAPIServce, { ScanAttributes } from '@/services/scans'
import QueueService, { Queues } from '@/services/queues'

import NotifyMixin from '@/mixins/notify'

let pollingInterval: number

interface DashboardAttributes {
  alertsLoading: boolean
  scansLoading: boolean
  queuesLoading: boolean
  colors: string[]
  queues: Queues
  scans: ScanAttributes[]
  alerts: AlertAttributes[]
}

export default (Vue as VueConstructor<Vue & DashboardAttributes>).extend({
  name: 'DashboardView',
  mixins: [NotifyMixin],
  data() {
    return {
      alertsLoading: false,
      scansLoading: false,
      queuesLoading: false,
      colors: Object.freeze({
        error: 'red',
        failed: 'red',
        running: 'orange',
        completed: 'green',
        active: 'yellow',
      }),
      queues: { schedule: 0, scanner: 0, event: 0 } as Queues,
      scans: [] as ScanAttributes[],
      alerts: [] as AlertAttributes[],
    }
  },
  methods: {
    getAlerts() {
      this.alertsLoading = true
      AlertAPIService.list({
        page: 1,
        pageSize: 5,
        orderColumn: 'created_at',
        orderDirection: 'desc',
      }).then((res) => {
        this.alerts = res.data.results
        this.alertsLoading = false
      })
    },
    getQueues() {
      this.queuesLoading = true
      QueueService.view()
        .then((res) => {
          this.queues = res.data
          this.queuesLoading = false
        })
        .catch(this.errorHandler)
    },
    getScans() {
      this.scansLoading = true
      ScanAPIServce.list({
        fields: ['id', 'created_at'],
        page: 1,
        no_test: true,
        pageSize: 5,
        orderColumn: 'created_at',
        eager: ['sites', 'sources'],
        orderDirection: 'desc',
      })
        .then((res) => {
          this.scans = res.data.results
        })
        .catch(this.errorHandler)
    },
    getAll() {
      this.getAlerts()
      this.getScans()
      this.getQueues()
    },
  },
  beforeDestroy() {
    clearInterval(pollingInterval)
  },
  created() {
    this.getAll()
    pollingInterval = setInterval(this.getAll, 5000)
  },
})
</script>
