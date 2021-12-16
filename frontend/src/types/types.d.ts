import 'vue-router'
import { Notifications } from './store'

declare module 'vue/types/vue' {
  interface Vue {
    errorHandler(error: unknown | Error): void
    notify(notification: Notifications): string
    info({ title: string, body: string }): string
  }
}

declare module 'vue-router' {
  interface RouteMeta {
    authorze?: string[]
    forwardAuth?: boolean
  }
}
