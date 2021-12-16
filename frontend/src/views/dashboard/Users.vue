<template>
  <v-container id="users" fluid tag="section">
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
              <v-toolbar-title>User Listing</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-btn
                color="primary"
                dark
                class="mb-2"
                @click="$router.push('/users/edit')"
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
                      path: '/users/edit',
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
            <confirm ref="confirm"></confirm>
          </template>
        </v-data-table>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'

import UserAPIService, { UserAttributes } from '../../services/user'
import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

import TableMixin, { TableMixinBindings } from '@/mixins/table'
import NotifyMixin from '@/mixins/notify'

const listFields: (keyof Partial<UserAttributes>)[] = [
  'id',
  'login',
  'role',
  'created_at',
]

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin, NotifyMixin],
  data() {
    return {
      options: {},
      headers: Object.freeze([
        {
          text: 'Login',
          align: 'start',
          sortable: true,
          value: 'login',
        },
        {
          text: 'Role',
          sortable: true,
          value: 'role',
        },
        {
          text: 'Created',
          sortable: true,
          value: 'created_at',
        },
        {
          text: 'Actions',
          value: 'actions',
          sortable: false,
        },
      ]),
      records: [] as UserAttributes[],
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
      const res = await UserAPIService.list({
        fields: listFields,
        page: this.page,
        pageSize: this.itemsPerPage,
        ...this.resolveOrder(),
      })
      this.loading = false
      this.records = res.data.results
      this.total = res.data.total
    },
    async deleteItem(id: string) {
      const dialog = (this.$refs.confirm as unknown) as ConfirmDialog
      const res = await dialog.open('Delete', 'Are you sure?', {
        color: 'red',
        width: 350,
      })
      if (res) {
        try {
          await UserAPIService.destroy({ id })
          this.info({ title: 'Users', body: 'User Deleted' })
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
