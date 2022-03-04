<template>
  <v-container id="sources" fluid tag="section">
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
            <v-toolbar flat>
              <v-toolbar-title>Puppeteer Sources</v-toolbar-title>
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
                    style="margin-right: 15px"
                  >
                    <v-icon dark>mdi-test-tube</v-icon>
                  </v-btn>
                </template>
                <span>Toggle Test</span>
              </v-tooltip>
              <v-btn
                color="primary"
                dark
                class="mb-2"
                @click="$router.push('/source/edit')"
              >
                New
              </v-btn>
            </v-toolbar>
          </template>

          <template v-slot:[`item.actions`]="{ item }">
            <v-tooltip bottom>
              <template v-slot:activator="{ on, attrs }">
                <v-icon
                  small
                  class="mr-2"
                  v-bind="attrs"
                  v-on="on"
                  @click="copyItem(item)"
                >
                  mdi-content-copy
                </v-icon>
              </template>
              <span>Copy</span>
            </v-tooltip>
            <v-tooltip bottom>
              <template v-slot:activator="{ on, attrs }">
                <v-btn
                  icon
                  color="red"
                  :disabled="item.scans.length > 0"
                  @click="deleteItem(item.id)"
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

import SourceAPIService, { SourceAttributes } from '../../services/sources'
import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

import TableMixin, { TableMixinBindings } from '@/mixins/table'
import NotifyMixin from '@/mixins/notify'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  name: 'SourcesView',
  mixins: [TableMixin, NotifyMixin],
  data() {
    return {
      showTest: false,
      options: {},
      headers: Object.freeze([
        {
          text: 'Name',
          align: 'start',
          sortable: true,
          value: 'name'
        },
        {
          text: 'Scans',
          value: 'scans.length'
        },
        {
          text: 'Created',
          value: 'created_at'
        },
        {
          text: 'Actions',
          value: 'actions',
          sortable: false
        }
      ]),
      records: [] as SourceAttributes[]
    }
  },
  watch: {
    options: {
      handler() {
        this.$nextTick(() => {
          this.list()
        })
      },
      deep: true
    },
    showTest() {
      this.list()
    }
  },
  methods: {
    async list() {
      const res = await SourceAPIService.list({
        fields: ['id', 'name', 'created_at'],
        page: this.page,
        pageSize: this.itemsPerPage,
        eager: ['scans'],
        no_test: !this.showTest,
        ...this.resolveOrder()
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    copyItem(item: SourceAttributes) {
      this.$router.push({ path: '/source/edit', query: { id: item.id } })
    },
    async deleteItem(id: string) {
      // yuck
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Delete', 'Are you sure?', {
        color: 'red',
        width: 350
      })
      if (res) {
        await SourceAPIService.destroy({ id })
          .then(() => {
            this.info({ title: 'Source', body: 'Source Deleted' })
            this.list()
          })
          .catch(this.errorHandler)
      }
    }
  },
  components: {
    Confirm
  }
})
</script>
