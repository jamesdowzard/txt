import { defineConfig } from 'vite';
import preact from '@preact/preset-vite';
import UnoCSS from 'unocss/vite';
import { presetMini } from '@unocss/preset-mini';
import { resolve } from 'path';

export default defineConfig({
  plugins: [
    preact(),
    UnoCSS({ presets: [presetMini()] }),
  ],
  build: {
    outDir: resolve(__dirname, '../internal/web/static/dist'),
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: resolve(__dirname, 'index.html'),
        popover: resolve(__dirname, 'popover.html'),
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '^/(api|messages|conversations|contacts|mcp|events|media|outbox|search|debug|insights)': {
        target: 'http://127.0.0.1:7007',
        changeOrigin: true,
      },
      '^/favicon\\.svg': {
        target: 'http://127.0.0.1:7007',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'happy-dom',
  },
});
