import Vue from 'vue'
import { Notifications } from '../store'
import { mapMutations } from 'vuex'
import axios from 'axios'

const notificationTypes = {
  error: 'red--text',
  info: 'blue--text',
  warning: 'orange--text',
}

export interface NotifyAttributes {
  notifyColor: string
  notifyTitle: string
  notifyBody: string
  toggleSnackbar: boolean
}

export default Vue.extend({
  data: () => ({
    notifyColor: '',
    notifyTitle: '',
    notifyBody: '',
    toggleSnackbar: false,
  }),
  methods: {
    ...mapMutations({
      setNotifications: 'setNotifications',
    }),
    errorHandler(e: unknown | Error) {
      if (!axios.isAxiosError(e)) {
        this.notify({
          type: 'error',
          title: 'Unknown error',
          body: `An unknown error occured`,
        })
        return
      }
      let title = 'Unknown Error'
      let body = e.message
      if (e.response) {
        title = e.response.data.message
        body = e.response.data?.data?.reason
      }
      this.notify({
        type: 'error',
        title,
        body,
      })
    },
    notify(notification: Notifications) {
      this.setNotifications(notification)
    },
    info({ title, body }: { title: string; body: string }) {
      this.notify({ type: 'info', title, body })
    },
  },
  mounted() {
    this.$store.subscribe((mutation) => {
      if (mutation.type === 'setNotifications' && mutation.payload) {
        this.toggleSnackbar = true
        this.notifyTitle = mutation.payload.title
        this.notifyBody = mutation.payload.body
        const type: keyof typeof notificationTypes = mutation.payload.type
        this.notifyColor = notificationTypes[type]
      }
    })
  },
})
