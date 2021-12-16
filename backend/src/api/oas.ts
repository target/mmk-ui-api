/* eslint-disable @typescript-eslint/explicit-module-boundary-types */
import { PathItem } from 'aejo'

export default (paths: PathItem) => ({
  openapi: '3.0.0',
  info: {
    version: '2.0.0',
    title: 'MerryMaker',
    description: 'MerryMaker API Schema',
  },
  paths,
})
