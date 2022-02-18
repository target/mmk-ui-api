let { HOST, API_HOST } = process.env
let PORT = process.env.PORT && Number(process.env.PORT)
let MMK_PORT = process.env.MMK_PORT && Number(process.env.MMK_PORT)

HOST = HOST || '0.0.0.0'
API_HOST = API_HOST || HOST
PORT = PORT || 8080
MMK_PORT = MMK_PORT || 3030

module.exports = {
  transpileDependencies: [
    'vuetify'
  ],
  devServer: {
    hot: true,
    proxy: {
      '/api': {
        target: `http://${API_HOST}:${MMK_PORT}`
      }
    },
    host: HOST,
    port: PORT,
  },
  chainWebpack: config => {
    config
      .plugin('html')
      .tap(args => {
        args[0].title = 'MerryMaker'
        return args
      })
  }
}
