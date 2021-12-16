<template>
  <v-container id="secrets-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>Secrets Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col col="8" md="4">
                  <v-text-field
                    :disabled="id.length > 0"
                    label="Name"
                    tabindex="-1"
                    v-model="name"
                    required
                  >
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row>
                <v-col cols="12" md="6">
                  <v-textarea
                    label="Secret"
                    v-model="value"
                    required
                  ></v-textarea>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="5" md="3">
                  <v-radio-group v-model="type">
                    <template v-for="sType of activeTypes">
                      <v-radio
                        :key="sType.key"
                        :label="sType.label"
                        :value="sType.key"
                      >
                      </v-radio>
                    </template>
                  </v-radio-group>
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
                    @click="$router.push({ path: '/secrets' })"
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
import SecretAPIService, { SecretTypes } from '@/services/secrets'

import NotifyMixin from '../../mixins/notify'

const typeMapping = [
  { key: 'manual', label: 'Static' },
  { key: 'qt', label: 'Quantum Tunnel' },
]

export default Vue.extend({
  name: 'SecretForm',
  mixins: [NotifyMixin],
  data() {
    return {
      id: '',
      name: '',
      value: '',
      type: 'manual' as SecretTypes,
      activeTypes: [] as typeof typeMapping,
      loading: false,
      action: 'Save',
    }
  },

  methods: {
    async submit() {
      try {
        if (this.id !== '') {
          await SecretAPIService.update(this.id, {
            type: this.type,
            value: this.value,
          })
        } else {
          await SecretAPIService.create({
            type: this.type,
            name: this.name,
            value: this.value,
          })
        }
        this.$router.push('/secrets')
      } catch (e) {
        this.errorHandler(e)
      }
    },
    getSecret(id: string) {
      SecretAPIService.view({ id })
        .then((res) => {
          this.name = res.data.name
          this.type = res.data.type
          this.value = res.data.value
        })
        .catch(this.errorHandler)
    },
  },
  created() {
    SecretAPIService.types().then((result) => {
      this.activeTypes = typeMapping.filter((typ) =>
        result.data.types.includes(typ.key as SecretTypes)
      )
    })
    if (this.$route.query.id && typeof this.$route.query.id === 'string') {
      this.id = this.$route.query.id
      this.getSecret(this.id)
    }
  },
})
</script>
