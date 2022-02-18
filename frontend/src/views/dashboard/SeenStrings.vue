<template>
  <v-container id="SeenString" fluid tag="section">
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
              <v-col cols="4">
                <v-text-field
                  color="secondary"
                  hide-details
                  label="Search"
                  v-model="search"
                  class="mx-4"
                  single-line
                  @keyup.enter="list"
                >
                  <template v-slot:append-outer>
                    <v-btn class="mt-n2" elevation="1" fab small @click="list">
                      <v-icon>mdi-magnify</v-icon>
                    </v-btn>
                  </template>
                </v-text-field>
              </v-col>
              <v-col cols="2">
                <v-select
                  v-model="typeFilter"
                  :items="seenStringTypes"
                  chips
                  label="Types"
                >
                </v-select>
              </v-col>
              <template v-if="role === 'admin'">
                <v-spacer></v-spacer>
                <v-btn
                  color="primary"
                  dark
                  @click="$router.push('/seen_strings/edit')"
                >
                  New
                </v-btn>
              </template>
            </v-toolbar>
          </template>
          <template v-slot:[`item.actions`]="{ item }" v-if="role === 'admin'">
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
                      path: '/seen_strings/edit',
                      query: { id: item.id }
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
import store from '@/store'
import SeenStringAPIService, {
  SeenStringAttributes
} from '@/services/seen_strings'
import TableMixin, { TableMixinBindings } from '@/mixins/table'
import NotifyMixin from '@/mixins/notify'

import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin, NotifyMixin],
  name: 'SeenStringsView',
  data() {
    return {
      options: {},
      seenStringTypes: [] as string[],
      typeFilter: '',
      search: '',
      headers: Object.freeze([
        {
          text: 'key',
          align: 'start',
          sortable: true,
          value: 'key'
        },
        {
          text: 'type',
          align: 'start',
          sortable: true,
          value: 'type'
        },
        {
          text: 'Created',
          sortable: true,
          value: 'created_at'
        },
        {
          text: 'Actions',
          value: 'actions',
          sortable: false
        }
      ]),
      records: [] as SeenStringAttributes[]
    }
  },
  computed: {
    role: () => store.getters.user.role
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
    typeFilter() {
      this.$nextTick(() => {
        this.list()
      })
    }
  },
  methods: {
    async list() {
      const res = await SeenStringAPIService.list({
        page: this.page,
        pageSize: this.itemsPerPage,
        type: this.typeFilter.length ? this.typeFilter : undefined,
        search: this.search.length ? this.search : undefined,
        ...this.resolveOrder()
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    async getDistinct() {
      try {
        const res = await SeenStringAPIService.distinct({ column: 'type' })
        this.seenStringTypes = res.data.map(e => e.type)
      } catch (e) {
        this.errorHandler(e)
      }
    },
    async deleteItem(id: string) {
      // yuck yuck
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Delete', 'Are you sure?', {
        color: 'red',
        width: 350
      })
      if (res) {
        try {
          await SeenStringAPIService.destroy({ id })
          this.info({ title: 'Seen String', body: 'Seen String Deleted' })
          await this.list()
        } catch (e) {
          console.error(e)
        }
      }
    }
  },
  created() {
    this.getDistinct()
  },
  components: {
    Confirm
  }
})
</script>
