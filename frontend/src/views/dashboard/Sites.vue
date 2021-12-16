<template>
  <v-container id="sites" fluid tag="section">
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
              <v-toolbar-title>Site Listing</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-btn
                color="primary"
                dark
                class="mb-2"
                @click="$router.push({ name: 'SiteForm' })"
              >
                New
              </v-btn>
            </v-toolbar>
          </template>
          <template v-slot:[`item.name`]="{ item }">
            <router-link
              :to="{ name: 'Site', params: { id: item.id } }"
              style="text-decoration: none; color: inherit"
              >{{ item.name }}
            </router-link>
          </template>
          <template v-slot:[`item.actions`]="{ item }">
            <v-tooltip bottom>
              <template v-slot:activator="{ on, attrs }">
                <v-icon
                  small
                  color="primary"
                  class="mr-2"
                  v-bind="attrs"
                  v-on="on"
                  @click="editItem(item)"
                >
                  mdi-pencil
                </v-icon>
              </template>
              <span>Edit</span>
            </v-tooltip>
            <v-tooltip bottom>
              <template v-slot:activator="{ on, attrs }">
                <v-btn icon color="red" @click="deleteItem(item.id)">
                  <v-icon small v-bind="attrs" v-on="on"> mdi-delete </v-icon>
                </v-btn>
              </template>
              <span>Delete</span>
            </v-tooltip>
            <confirm ref="confirm"></confirm>
          </template>
        </v-data-table>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'

import SiteAPIService, { SiteAttributes } from '../../services/sites'
import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

import TableMixin, { TableMixinBindings } from '@/mixins/table'

import NotifyMixin from '@/mixins/notify'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin, NotifyMixin],
  data() {
    return {
      options: {},
      headers: Object.freeze([
        {
          text: 'Name',
          align: 'start',
          sortable: true,
          value: 'name',
        },
        {
          text: 'Active',
          sortable: true,
          value: 'active',
        },
        {
          text: 'Last Run',
          value: 'last_run',
        },
        {
          text: 'Last Enabled',
          value: 'last_enabled',
          sortable: true,
        },
        {
          text: 'Actions',
          value: 'actions',
          sortable: false,
        },
      ]),
      records: [] as SiteAttributes[],
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
  },
  methods: {
    async list() {
      this.loading = true
      const res = await SiteAPIService.list({
        fields: ['id', 'name', 'active', 'last_run'],
        page: this.page,
        pageSize: this.itemsPerPage,
        ...this.resolveOrder(),
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    editItem(item: SiteAttributes) {
      this.$router.push({ name: 'SiteForm', params: { id: item.id } })
    },
    async deleteItem(id: string) {
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Delete', 'Are you sure?', {
        color: 'red',
        width: 350,
      })
      if (res) {
        try {
          await SiteAPIService.destroy({ id })
          this.info({ title: 'Sites', body: 'Site Deleted' })
          await this.list()
        } catch (e) {
          console.error(e)
        }
      }
    },
  },
  components: {
    Confirm,
  },
})
</script>
