<template>
  <v-container id="seen_strings-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>Seen Strings Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col cols="12" md="6">
                  <v-textarea label="Key" v-model="key" required></v-textarea>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="5" md="3">
                  <v-radio-group v-model="type" label="Type">
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
                    @click="$router.push({ path: '/seen_strings' })"
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
import SeenStringsAPIService from '@/services/seen_strings'
import NotifyMixin from '../../mixins/notify'

const typeMapping = [
  { key: 'domain', label: 'Domain' },
  { key: 'hash', label: 'Hash' },
  { key: 'email', label: 'Email' },
  { key: 'url', label: 'URL' },
]

export default Vue.extend({
  name: 'SeenStringForm',
  mixins: [NotifyMixin],
  data() {
    return {
      id: '',
      key: '',
      type: '',
      action: 'Save',
      loading: false,
      activeTypes: Object.freeze(typeMapping),
    }
  },
  methods: {
    async submit() {
      try {
        if (this.id !== '') {
          await SeenStringsAPIService.update(this.id, {
            type: this.type,
            key: this.key
          })
        } else {
          await SeenStringsAPIService.create({
            type: this.type,
            key: this.key
          })
        }
        this.info({
          title: 'Seen String',
          body: `${this.type} saved`,
        })
        this.$router.push('/seen_strings')
      } catch (e) {
        this.errorHandler(e)
      }
    },
    async getSeenString(id: string) {
      try {
        const res = await SeenStringsAPIService.view({ id })
        this.type = res.data.type
        this.key = res.data.key
      } catch (e) {
        this.errorHandler(e)
      }
    },
  },
  async created() {
    const { id } = this.$route.query
    if (typeof id === 'string') {
      this.id = id
      await this.getSeenString(this.id)
    }
  }
})
</script>
