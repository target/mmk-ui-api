<template>
  <v-container id="ioc-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>IOC Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col col="8" md="4">
                  <v-text-field
                    v-if="!bulk"
                    label="Value"
                    tabindex="-1"
                    v-model="value"
                    required
                  >
                  </v-text-field>
                  <v-textarea
                    tabindex="-1"
                    v-model="value"
                    hint="line-separated values"
                    v-else
                    label="Bulk IOCs"
                  ></v-textarea>
                  <v-switch
                    :disabled="!isNew"
                    v-model="bulk"
                    :label="`Bulk Values`"
                  >
                  </v-switch>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="5" md="3">
                  <v-select
                    v-model="type"
                    :items="iocTypes"
                    label="Type"
                    required
                  ></v-select>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="4">
                  <v-checkbox v-model="enabled" label="Enabled"></v-checkbox>
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
                    @click="$router.push({ path: '/iocs' })"
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
import IocAPIService, { IocType, IocRequest } from '@/services/iocs'

import NotifyMixin from '../../mixins/notify'

export default Vue.extend({
  name: 'IocForm',
  mixins: [NotifyMixin],
  data() {
    return {
      id: '',
      value: '',
      type: 'fqdn',
      enabled: true,
      loading: false,
      action: 'Save',
      isNew: true,
      bulk: false,
      iocTypes: Object.freeze(['fqdn', 'ip', 'literal']),
    }
  },
  methods: {
    async submit() {
      const payload: IocRequest = {
        ioc: {
          value: this.value,
          type: this.type as IocType,
          enabled: this.enabled,
        },
      }
      try {
        if (this.id !== '') {
          await IocAPIService.update(this.id, payload)
        } else if (this.bulk) {
          await IocAPIService.bulkCreate({
            iocs: {
              type: payload.ioc.type,
              enabled: payload.ioc.enabled,
              values: this.value.split(/\r\n|[\n\v\f\r\x85\u2028\u2029]/),
            },
          })
        } else {
          await IocAPIService.create(payload)
        }
        this.$router.push('/iocs')
      } catch (e) {
        this.errorHandler(e)
      }
    },
    getIoc(id: string) {
      IocAPIService.view({ id })
        .then((res) => {
          this.value = res.data.value
          this.type = res.data.type
          this.enabled = res.data.enabled
        })
        .catch(this.errorHandler)
    },
  },
  created() {
    if (this.$route.query.id && typeof this.$route.query.id === 'string') {
      this.id = this.$route.query.id
      this.isNew = false
      this.getIoc(this.id)
    }
  },
})
</script>
