<template>
  <v-container>
    <v-layout row justify-space-around>
      <v-row>
        <v-col cols="8">
          <v-card height="400">
            <v-toolbar flat>
              <v-toolbar-title>Site Flows</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-tooltip bottom>
                <template v-slot:activator="{ on, attrs }">
                  <v-btn color="primary" dark v-bind="attrs" v-on="on" icon>
                    <v-icon>mdi-plus</v-icon>
                  </v-btn>
                </template>
                <span>New Site</span>
              </v-tooltip>
            </v-toolbar>
            <v-card-text>
              <v-simple-table>
                <template v-slow:default>
                  <thead>
                    <tr>
                      <th>Site</th>
                      <th>Enabled</th>
                      <th>Status</th>
                      <th>Last Scan Completed</th>
                    </tr>
                  </thead>
                </template>
              </v-simple-table>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col cols="4">
          <v-card height="400">
            <v-toolbar flat>
              <v-toolbar-title>Alerts</v-toolbar-title>
            </v-toolbar>
          </v-card>
        </v-col>
      </v-row>
    </v-layout>
  </v-container>
</template>
