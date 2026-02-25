import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

const parsedPort = Number(process.env.VITE_DEV_PORT ?? '')
const hasCustomPort = Number.isInteger(parsedPort) && parsedPort > 0

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    ...(hasCustomPort ? { port: parsedPort } : {}),
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    minify: 'esbuild',
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
    css: true,
    clearMocks: true,
  },
})
