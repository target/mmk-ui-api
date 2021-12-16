<template>
  <v-app>
    <v-snackbar v-model="toggleSnackbar">
      <strong :class="notifyColor">{{ notifyTitle }}</strong>
      <br />
      <span>{{ notifyBody }}</span>
      <template v-slot:action="{ attrs }">
        <v-btn color="red" text v-bind="attrs" @click="toggleSnackbar = false">
          Close
        </v-btn>
      </template>
    </v-snackbar>
    <span class="login-bg"></span>
    <v-card class="overflow-hidden">
      <v-app-bar flat dark>
        <v-toolbar-title>Merry Maker 2</v-toolbar-title>
      </v-app-bar>
    </v-card>
    <v-container fill-height>
      <v-row>
        <v-col col="12" lg="12" align="center">
          <v-card class="mx-auto" max-width="400">
            <v-card-title class="text-h5">
              <v-icon color="red"> mdi-lock </v-icon>
              User Login
            </v-card-title>
            <v-card-text
              class="justify-center text--primary"
              v-if="strategy === 'oauth'"
            >
              <v-container>
                <v-btn href="/api/auth/oauth" v-if="ready">OAuth2</v-btn>
                <p v-else>
                  <v-icon color="red"> mdi-alert </v-icon>
                  Invalid/Missing OAuth Configuration
                </p>
              </v-container>
            </v-card-text>
            <v-card-text
              class="justify-center text--primary"
              v-if="strategy === 'local'"
            >
              <v-form ref="loginForm" @submit.prevent="login" v-if="ready">
                <v-container>
                  <v-row>
                    <v-col>
                      <v-text-field
                        label="Login"
                        autofocus
                        tabindex="-1"
                        v-model="user.login"
                        required
                        :disabled="busy"
                      >
                      </v-text-field>
                    </v-col>
                  </v-row>
                  <v-row>
                    <v-col>
                      <v-text-field
                        label="Password"
                        v-model="user.password"
                        :type="'password'"
                        :disabled="busy"
                        required
                      >
                      </v-text-field>
                    </v-col>
                  </v-row>
                </v-container>
                <v-card-actions>
                  <v-btn text color="primary" type="submit" v-if="!busy">
                    Login
                  </v-btn>
                  <v-progress-circular
                    :width="3"
                    color="primary"
                    indeterminate
                    v-else
                  ></v-progress-circular>
                </v-card-actions>
              </v-form>
              <v-form
                @submit.prevent="createAdmin"
                v-if="!ready"
                ref="createAdmin"
              >
                <v-container>
                  <h3>Merry Maker Setup</h3>
                  <p class="text-justify">
                    Please set the <code>admin</code> password to continue
                  </p>
                  <v-row>
                    <v-col>
                      <v-text-field
                        label="Login"
                        :disabled="true"
                        value="admin"
                      >
                      </v-text-field>
                    </v-col>
                  </v-row>
                  <v-row>
                    <v-col>
                      <v-text-field
                        label="Password"
                        tabindex="-1"
                        v-model="user.password"
                        :type="'password'"
                        :rules="passwordRules"
                        required
                        :disabled="busy"
                      >
                      </v-text-field>
                    </v-col>
                  </v-row>
                  <v-row>
                    <v-col>
                      <v-text-field
                        label="Confirm Password"
                        v-model="confirm"
                        :type="'password'"
                        :rules="confirmPasswordRule"
                        :disabled="busy"
                        required
                      >
                      </v-text-field>
                    </v-col>
                  </v-row>
                </v-container>
                <v-card-actions>
                  <v-btn text color="primary" type="submit" v-if="!busy">
                    Create
                  </v-btn>
                  <v-progress-circular
                    :width="3"
                    color="primary"
                    indeterminate
                    v-else
                  ></v-progress-circular>
                </v-card-actions>
              </v-form>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </v-container>
  </v-app>
</template>

<script lang="ts">
import Vue, { VueConstructor } from 'vue'
import AuthAPIService from '@/services/auth'
import UserAPIService from '@/services/user'

import NotifyMixin, { NotifyAttributes } from '../mixins/notify'

// https://github.com/vuejs/vue/issues/8721
interface UserLoginAttrs extends NotifyAttributes {
  strategy: string
  ready: boolean
  user: {
    login: string
    password: string
  }
  $refs: {
    createAdmin: {
      validate: () => boolean
    }
  }
  getReady: () => void
}

export default (Vue as VueConstructor<Vue & UserLoginAttrs>).extend({
  name: 'Login',
  mixins: [NotifyMixin],
  data() {
    return {
      strategy: 'local',
      validForm: false,
      ready: false,
      busy: false,
      passwordRules: [
        (v: string) =>
          v.length >= 8 || 'Password must be at least 8 characters',
      ],
      user: {
        login: '',
        password: '',
      },
      confirm: '',
    }
  },
  computed: {
    confirmPasswordRule() {
      return [
        (v: string) => v === this.user.password || 'Passwords do not match',
      ]
    },
  },
  methods: {
    getReady() {
      AuthAPIService.ready()
        .then((res) => {
          this.ready = res.data.ready
          this.strategy = res.data.strategy
        })
        .catch(this.errorHandler)
    },
    login() {
      this.busy = true
      AuthAPIService.login({ user: this.user })
        .then(() => {
          window.location.reload()
        })
        .catch(this.errorHandler)
        .finally(() => {
          this.busy = false
        })
    },
    createAdmin() {
      if (!this.$refs.createAdmin.validate()) {
        return
      }
      UserAPIService.createAdmin({ password: this.user.password })
        .then(() => {
          window.location.reload()
        })
        .catch(this.errorHandler)
        .finally(() => {
          this.busy = false
        })
    },
  },
  beforeMount() {
    this.getReady()
  },
})
</script>

<style scoped>
.login-bg {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
  background: url('~@/assets/tunnel.webp');
  background-size: cover;
  transform: scale(1.1);
}
</style>
