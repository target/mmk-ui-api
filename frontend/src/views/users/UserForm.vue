<template lang="html">
  <v-container id="user-form" fluid tag="section">
    <v-row justify="center">
      <v-col cols="12">
        <v-card class="px-5 py-3">
          <v-toolbar flat>
            <v-toolbar-title>User Editor</v-toolbar-title>
          </v-toolbar>
          <v-form>
            <v-container>
              <v-row>
                <v-col col="8" md="4">
                  <v-text-field
                    :disabled="loading"
                    label="Login"
                    tabindex="-1"
                    autofocus
                    v-model="login"
                    required
                  >
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="5" md="1">
                  <v-select
                    :disabled="loading"
                    v-model="role"
                    :items="roleTypes"
                    label="Role"
                    required
                  ></v-select>
                </v-col>
              </v-row>
              <v-row>
                <v-col col="8" md="4">
                  <v-text-field
                    :disabled="loading"
                    label="Password"
                    v-model="password"
                    type="password"
                  >
                  </v-text-field>
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
                    @click="$router.push({ path: '/users' })"
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
import UserAPIService, { UserAttributes, UserRole } from '@/services/user'
import NotifyMixin from '@/mixins/notify'

export default Vue.extend({
  name: 'UserForm',
  mixins: [NotifyMixin],
  data() {
    return {
      id: '',
      login: '',
      role: '',
      password: '',
      loading: false,
      action: 'Save',
      roleTypes: Object.freeze(['admin', 'user', 'transport']),
    }
  },
  methods: {
    async submit() {
      this.loading = true
      const payload: UserAttributes = {
        login: this.login,
        role: this.role as UserRole,
      }
      if (this.password.length) {
        payload.password = this.password
      }
      try {
        if (this.id !== '') {
          await UserAPIService.update({ id: this.id, user: payload })
        } else {
          await UserAPIService.create({ user: payload })
        }
        this.$router.push('/users')
      } catch (e) {
        this.errorHandler(e)
      } finally {
        this.loading = false
      }
    },
    async getUser(id: string) {
      this.loading = true
      try {
        const res = await UserAPIService.view({ id })
        this.login = res.data.login
        this.role = res.data.role
        this.loading = false
      } catch (e) {
        this.errorHandler(e)
      }
    },
  },
  created() {
    if (this.$route.query.id && typeof this.$route.query.id === 'string') {
      this.id = this.$route.query.id
      this.getUser(this.id)
    }
  },
})
</script>
