<template>
  <v-container id="AllowList" fluid tag="section">
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
              <v-toolbar-title>Allow List</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-btn
                color="primary"
                dark
                class="mb-2"
                @click="$router.push('/allow_list/edit')"
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
                  color="primary"
                  class="mr-2"
                  v-bind="attrs"
                  v-on="on"
                  @click="
                    $router.push({
                      path: '/allow_list/edit',
                      query: { id: item.id },
                    })
                  "
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
          </template>
        </v-data-table>
      </v-col>
    </v-row>
    <confirm ref="confirm"></confirm>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'

import AllowListAPIService, {
  AllowListAttributes,
} from '../../services/allow_list'
import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

import TableMixin, { TableMixinBindings } from '@/mixins/table'

import NotifyMixin from '@/mixins/notify'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin, NotifyMixin],
  name: 'AllowListView',
  data() {
    return {
      options: {},
      headers: Object.freeze([
        {
          text: 'Key',
          align: 'start',
          sortable: true,
          value: 'key',
        },
        {
          text: 'Type',
          sortable: true,
          value: 'type',
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
      records: [] as AllowListAttributes[],
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
      const res = await AllowListAPIService.list({
        page: this.page,
        pageSize: this.itemsPerPage,
        ...this.resolveOrder(),
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    async deleteItem(id: string) {
      // yuck yuck
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Delete', 'Are you sure?', {
        color: 'red',
        width: 350,
      })
      if (res) {
        try {
          await AllowListAPIService.destroy({ id })
          this.info({ title: 'Allow List', body: 'Item Deleted' })
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
