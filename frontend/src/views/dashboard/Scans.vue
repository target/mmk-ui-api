<template>
  <v-container id="scans" fluid tag="section">
    <v-row>
      <v-col cols="12">
        <v-data-table
          v-model="selected"
          :headers="headers"
          :items="records"
          :options.sync="options"
          :server-items-length="total"
          :page.sync="page"
          show-select
          :sort-by.sync="sortBy"
          :sort-desc.sync="sortDesc"
          :loading="loading"
          :items-per-page.sync="itemsPerPage"
          :footer-props="{ itemsPerPageOptions: [10, 25, 50, 100, -1] }"
          class="elevation-1"
          @page-count="pageCount = $event"
        >
          <template v-slot:top>
            <v-toolbar flat>
              <v-toolbar-title>
                <v-tooltip bottom>
                  <template v-slot:activator="{ on, attrs }">
                    <v-btn
                      color="error"
                      :disabled="selected.length === 0"
                      x-small
                      icon
                      v-bind="attrs"
                      v-on="on"
                      @click="bulkDelete"
                    >
                      <v-icon dark> mdi-delete </v-icon>
                    </v-btn>
                  </template>
                  <span>Bulk Delete</span>
                </v-tooltip>
                Scans
              </v-toolbar-title>
              <v-spacer></v-spacer>
              <v-tooltip bottom>
                <template v-slot:activator="{ on, attrs }">
                  <v-btn
                    @click="showTest = !showTest"
                    v-bind="attrs"
                    v-on="on"
                    x-small
                    icon
                    :color="showTest ? 'secondary' : ''"
                    style="margin-right: 5px"
                  >
                    <v-icon dark>mdi-test-tube</v-icon>
                  </v-btn>
                </template>
                <span>Toggle Test</span>
              </v-tooltip>
            </v-toolbar>
          </template>

          <template v-slot:[`item.source.name`]="{ item }">
            <router-link
              :to="{ name: 'ScanLog', params: { id: item.id } }"
              style="text-decoration: none; color: inherit"
            >
              {{ item.source.name }}
            </router-link>
          </template>
          <template v-slot:[`item.actions`]="{ item }">
            <v-tooltip bottom>
              <template v-slot:activator="{ on, attrs }">
                <v-btn
                  icon
                  color="red"
                  @click="deleteItem(item.id)"
                  :disabled="item.state === 'active'"
                >
                  <v-icon small v-bind="attrs" v-on="on"> mdi-delete </v-icon>
                </v-btn>
              </template>
              <span>Delete</span>
            </v-tooltip>
          </template>
        </v-data-table>
      </v-col>
    </v-row>

    <confirm ref="confirm"></confirm>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'

import ScanAPIService, { ScanAttributes } from '../../services/scans'
import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

import TableMixin, { TableMixinBindings } from '@/mixins/table'

import NotifyMixin from '@/mixins/notify'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin, NotifyMixin],
  data() {
    return {
      showTest: false,
      options: {},
      selected: [],
      headers: Object.freeze([
        {
          text: 'Source',
          align: 'start',
          sortable: true,
          value: 'source.name',
        },
        {
          text: 'State',
          value: 'state',
          sortable: true,
        },
        {
          text: 'Test',
          value: 'test',
          sortable: true,
        },
        {
          text: 'Created',
          value: 'created_at',
        },
        {
          text: 'Actions',
          value: 'actions',
          sortable: false,
        },
      ]),
      records: [] as ScanAttributes[],
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
    showTest() {
      this.list()
    },
  },
  methods: {
    async list() {
      this.selected = []
      const res = await ScanAPIService.list({
        fields: ['id', 'name', 'created_at'],
        page: this.page,
        pageSize: this.itemsPerPage,
        eager: ['sites', 'sources'],
        no_test: !this.showTest,
        ...this.resolveOrder(),
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    async bulkDelete() {
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Bulk Delete', 'Are you sure?', {
        color: 'red',
        width: 350,
      })
      if (res) {
        await ScanAPIService.bulkDelete({
          ids: this.selected.map((s: ScanAttributes) => s.id),
        })
          .then(this.list)
          .catch(this.errorHandler)
      }
    },
    async deleteItem(id: string) {
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Delete', 'Are you sure?', {
        color: 'red',
        width: 350,
      })
      if (res) {
        try {
          await ScanAPIService.destroy({ id })
          this.info({ title: 'Scans', body: 'Scan Deleted' })
          await this.list()
        } catch (e) {
          this.errorHandler(e)
        }
      }
    },
  },
  components: {
    Confirm,
  },
})
</script>
