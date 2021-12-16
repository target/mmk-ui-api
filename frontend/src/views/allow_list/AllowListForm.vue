<template>
  <v-container id="allow-list-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>Allow List Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col col="8" md="4">
                  <v-text-field
                    label="Key"
                    tabindex="-1"
                    v-model="key"
                    required
                  >
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="5" md="3">
                  <v-select
                    v-model="type"
                    :items="allowListTypes"
                    label="Type"
                    required
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
                    @click="$router.push({ path: '/allow_list' })"
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
import AllowListAPIService, {
  AllowListType,
  AllowListRequest,
} from '@/services/allow_list'

import NotifyMixin from '../../mixins/notify'

export default Vue.extend({
  name: 'AllowListForm',
  mixins: [NotifyMixin],
  data() {
    return {
      id: '',
      key: '',
      type: 'fqdn',
      loading: false,
      action: 'Save',
      allowListTypes: Object.freeze([
        'fqdn',
        'ip',
        'literal',
        'ioc-payload-domain',
        'google-analytics',
      ]),
    }
  },
  methods: {
    async submit() {
      const payload: AllowListRequest = {
        key: this.key,
        type: this.type as AllowListType,
      }
      try {
        if (this.id !== '') {
          await AllowListAPIService.update(this.id, payload)
        } else {
          await AllowListAPIService.create(payload)
        }
        this.$router.push('/allow_list')
      } catch (e) {
        this.errorHandler(e)
      }
    },
    getAllowList(id: string) {
      AllowListAPIService.view({ id })
        .then((res) => {
          this.key = res.data.key
          this.type = res.data.type
        })
        .catch(this.errorHandler)
    },
  },
  created() {
    if (this.$route.query.id && typeof this.$route.query.id === 'string') {
      this.id = this.$route.query.id
      this.getAllowList(this.id)
    }
  },
})
</script>
