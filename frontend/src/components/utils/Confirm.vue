<template>
  <v-dialog
    v-model="show"
    :max-width="options.width"
    :style="{ zIndex: options.zIndex }"
    @keydown.esc="cancel"
  >
    <v-card>
      <v-toolbar :color="options.color" dark dense flat>
        <v-toolbar-title class="white--text">{{ title }}</v-toolbar-title>
      </v-toolbar>
      <v-card-text v-show="!!message" class="pa-4">{{ message }}</v-card-text>
      <v-card-actions class="pt-0">
        <v-spacer></v-spacer>
        <v-btn @click.native="agree" color="primary darken-1" text>Yes</v-btn>
        <v-btn @click.native="cancel" color="grey" text>Cancel</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import Vue from 'vue'

interface ConfirmOptions {
  color?: string
  width?: number
  zIndex?: number
}
export interface ConfirmDialog {
  open: (
    title: string,
    message: string,
    options: ConfirmOptions
  ) => Promise<boolean>
}

const ConfirmComponent = Vue.extend({
  name: 'ConfirmUtil',
  data() {
    return {
      dialog: false,
      resolve: (
        bool: PromiseLike<boolean> | boolean
      ): PromiseLike<boolean> | void | boolean => {
        return bool
      },
      reject: (
        bool: PromiseLike<boolean> | boolean
      ): PromiseLike<boolean> | void | boolean => {
        return bool
      },
      message: '',
      title: '',
      options: {
        color: 'primary',
        width: 290,
        zIndex: 200,
      } as ConfirmOptions,
    }
  },
  computed: {
    show: {
      get(): boolean {
        return this.dialog
      },
      set(value: boolean): void {
        this.dialog = value
        if (value === false) {
          this.cancel()
        }
      },
    },
  },
  methods: {
    open(
      title: string,
      message: string,
      options: ConfirmOptions
    ): Promise<boolean> {
      this.dialog = true
      this.title = title
      this.message = message
      this.options = { ...options }
      return new Promise((resolve, reject) => {
        this.resolve = resolve
        this.reject = reject
      })
    },
    agree() {
      this.resolve(true)
      this.dialog = false
    },
    cancel() {
      this.resolve(false)
      this.dialog = false
    },
  },
})

export default ConfirmComponent
</script>
