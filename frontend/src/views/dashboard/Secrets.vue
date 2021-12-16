<template>
  <v-container id="Secrets" fluid tag="section">
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
              <v-toolbar-title>Secrets</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-btn
                color="primary"
                dark
                class="mb-2"
                @click="$router.push('/secrets/edit')"
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
                      path: '/secrets/edit',
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
                <v-btn
                  icon
                  color="red"
                  @click="deleteItem(item.id)"
                  :disabled="item.sources.length > 0"
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

import SecretAPIService, { SecretAttributes } from '../../services/secrets'
import Confirm, { ConfirmDialog } from '../../components/utils/Confirm.vue'

import TableMixin, { TableMixinBindings } from '@/mixins/table'
import NotifyMixin from '@/mixins/notify'

export default (Vue as VueConstructor<Vue & TableMixinBindings>).extend({
  mixins: [TableMixin, NotifyMixin],
  name: 'Secrets',
  data() {
    return {
      loading: true,
      options: {},
      headers: Object.freeze([
        {
          text: 'Name',
          align: 'start',
          sortable: true,
          value: 'name',
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
          text: 'Updated',
          value: 'updated_at',
        },
        {
          text: 'Actions',
          value: 'actions',
          sortable: false,
        },
      ]),
      records: [] as SecretAttributes[],
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
      const res = await SecretAPIService.list({
        fields: ['id', 'name', 'type', 'created_at', 'updated_at'],
        page: this.page,
        eager: ['sources'],
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
          await SecretAPIService.destroy({ id })
          this.info({ title: 'Secrets', body: 'Secret Deleted' })
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
