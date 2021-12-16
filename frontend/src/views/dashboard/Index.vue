<template>
  <v-app>
    <dashboard-core-app-bar />
    <dashboard-core-drawer />
    <dashboard-core-view />
    <v-snackbar v-model="toggleSnackbar">
      <strong :class="notifyColor">{{ notifyTitle }}</strong>
      <br />
      <span>{{ notifyBody }}</span>
    </v-snackbar>
  </v-app>
</template>

<script lang="ts">
import Vue from 'vue'
import NotifyMixin from '@/mixins/notify'

export default Vue.extend({
  name: 'DashboardIndex',
  mixins: [NotifyMixin],
  components: {
    DashboardCoreAppBar: () => import('./components/core/AppBar.vue'),
    DashboardCoreDrawer: () => import('./components/core/Drawer.vue'),
    DashboardCoreView: () => import('./components/core/View.vue'),
  },
  data: () => ({
    expandOnHover: false,
  }),
})
</script>
