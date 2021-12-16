<template>
  <v-navigation-drawer
    id="core-navigation-drawer"
    v-model="drawer"
    :dark="barColor !== 'rgba(228, 226, 226, 1), rgba(255, 255, 255, 0.7)'"
    :expand-on-hover="expandOnHover"
    :right="$vuetify.rtl"
    :src="barImage"
    mobile-breakpoint="960"
    app
    width="260"
    v-bind="$attrs"
  >
    <template v-slot:img="props">
      <v-img :gradient="`to bottom, ${barColor}`" v-bind="props" />
    </template>

    <v-divider class="mb-1" />

    <v-list dense nav>
      <v-list-item>
        <v-list-item-avatar
          class="align-self-center"
          color="primary"
          size="42"
          contain
        >
          MMK
        </v-list-item-avatar>
        <v-list-item-content>
          <v-list-item v-text="profile.title" />
        </v-list-item-content>
      </v-list-item>
    </v-list>

    <v-divider class="mb-2" />

    <v-list expand nav>
      <div />
      <template v-for="(item, i) in items">
        <v-list-item
          :key="`item-${i}`"
          :to="item.to"
          v-if="user.role === 'admin' || user.role === item.role"
        >
          <v-list-item-icon>
            <v-icon v-text="item.icon"></v-icon>
          </v-list-item-icon>
          <v-list-item-content>
            <v-list-item-title v-text="item.title" />
          </v-list-item-content>
        </v-list-item>
      </template>
      <div />
    </v-list>
  </v-navigation-drawer>
</template>

<script lang="ts">
import Vue from 'vue'
import { mapState } from 'vuex'

export default Vue.extend({
  name: 'DashboardCoreDrawer',

  props: {
    expandOnHover: {
      type: Boolean,
      default: false,
    },
  },

  data: () => ({
    items: [
      {
        icon: 'mdi-view-dashboard',
        title: 'Dashboard',
        to: '/',
        role: 'user',
      },
      {
        icon: 'mdi-bell',
        title: 'Alerts',
        to: '/alerts',
        role: 'user',
      },
      {
        icon: 'mdi-web',
        title: 'Sites',
        to: '/sites',
        role: 'user',
      },
      {
        icon: 'mdi-key',
        title: 'Secrets',
        to: '/secrets',
        role: 'admin',
      },
      {
        icon: 'mdi-crosshairs',
        title: 'Scans',
        to: '/scans',
        role: 'admin',
      },
      {
        icon: 'mdi-clipboard-outline',
        title: 'Test Rules',
        to: '/test',
        role: 'admin',
      },
      {
        icon: 'mdi-code-string',
        title: 'Sources',
        to: '/sources',
        role: 'admin',
      },
      {
        icon: 'mdi-mushroom',
        title: 'IOCs',
        to: '/iocs',
        role: 'user',
      },
      {
        icon: 'mdi-check-all',
        title: 'Allow List',
        to: '/allow_list',
        role: 'user',
      },
      {
        icon: 'mdi-account',
        title: 'Users',
        to: '/users',
        role: 'admin',
      },
    ],
  }),
  computed: {
    ...mapState(['barColor', 'barImage']),
    drawer: {
      get() {
        return this.$store.state.drawer
      },
      set(val) {
        this.$store.commit('setDrawer', val)
      },
    },
    profile() {
      return {
        avatar: true,
        title: 'MerryMaker',
      }
    },
    user() {
      return this.$store.getters.user
    },
  },
})
</script>

<style lang="sass">
@import '~vuetify/src/styles/tools/_rtl.sass'
#core-navigation-drawer
  .v-list-group__header.v-list-item--active:before
    opacity: .24
  .v-list-item
    &__icon--text,
    &__icon:first-child
      justify-content: center
      text-align: center
      width: 20px
      +ltr()
        margin-right: 24px
        margin-left: 12px !important
      +rtl()
        margin-left: 24px
        margin-right: 12px !important
  .v-list--dense
    .v-list-item
      &__icon--text,
      &__icon:first-child
        margin-top: 10px
  .v-list-group--sub-group
    .v-list-item
      +ltr()
        padding-left: 8px
      +rtl()
        padding-right: 8px
    .v-list-group__header
      +ltr()
        padding-right: 0
      +rtl()
        padding-right: 0
      .v-list-item__icon--text
        margin-top: 19px
        order: 0
      .v-list-group__header__prepend-icon
        order: 2
        +ltr()
          margin-right: 8px
        +rtl()
          margin-left: 8px
</style>
