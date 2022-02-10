<template>
  <v-container id="site-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>Site Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col col="8" md="4">
                  <v-text-field
                    label="Name"
                    tabindex="1"
                    v-model="name"
                    required
                  >
                  </v-text-field>
                </v-col>
                <v-col col="4">
                  <v-checkbox v-model="active" label="Active"></v-checkbox>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="5" md="2">
                  <v-select
                    v-model="run_every_minutes"
                    :items="[5, 15, 30, 45, 60]"
                    label="Run every n-minutes"
                    required
                  ></v-select>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="12" md="3">
                  <v-select
                    v-model="source_id"
                    required
                    :items="sources"
                    item-text="name"
                    item-value="id"
                    label="Source"
                  ></v-select>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="12" md="1">
                  <v-btn color="primary" :disabled="loading" @click="submit">
                    {{ action }}
                  </v-btn>
                </v-col>
                <v-col md="1">
                  <v-btn
                    color="secondary"
                    :disabled="loading"
                    @click="$router.push({ path: '/sites' })"
                  >
                    Cancel
                  </v-btn>
                </v-col>
              </v-row>
            </v-container>
          </v-form>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts">
import Vue from 'vue'
import SourceAPIService, { SourceAttributes } from '../../services/sources'
import SiteAPIService, { SiteRequest } from '../../services/sites'

import NotifyMixin from '../../mixins/notify'

export default Vue.extend({
  name: 'SiteForm',
  mixins: [NotifyMixin],
  data() {
    return {
      name: '',
      id: '',
      sources: [] as Array<Partial<SourceAttributes>>,
      source_id: '',
      action: 'Save',
      run_every_minutes: 15,
      active: true,
      loading: false,
      showMessage: false,
      messageType: '',
      messageBody: '',
    }
  },
  methods: {
    async submit() {
      this.showMessage = false
      const payload: SiteRequest = {
        name: this.name,
        source_id: this.source_id,
        run_every_minutes: this.run_every_minutes,
        active: this.active,
      }
      try {
        if (this.id !== '') {
          await SiteAPIService.update(this.id, payload)
        } else {
          await SiteAPIService.create(payload)
        }
        this.$router.push('/sites')
      } catch (e) {
        this.errorHandler(e)
      }
    },
    getSources() {
      SourceAPIService.list({
        pageSize: 200,
        no_test: true,
        fields: ['id', 'name'],
        orderColumn: 'name',
      })
        .then((res) => {
          this.sources = res.data.results
        })
        .catch(this.errorHandler)
    },
    getSite(id: string) {
      SiteAPIService.view({ id })
        .then((res) => {
          this.name = res.data.name
          this.source_id = res.data.source_id
          this.run_every_minutes = res.data.run_every_minutes
          this.active = res.data.active
        })
        .catch(this.errorHandler)
    },
  },
  created() {
    if (this.$route.params.id && typeof this.$route.params.id === 'string') {
      this.id = this.$route.params.id
      this.getSite(this.id)
    }
    this.getSources()
  },
})
</script>
