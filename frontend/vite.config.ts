import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  server: {
    port: 18089,
    proxy: {
      '/api': {
        target: 'http://localhost:18088',
        changeOrigin: false,
        ws: true
      }
    }
  }
})
