import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: {
        BackupList: 'src/BackupList.tsx',
        BackupSettings: 'src/BackupSettings.tsx',
      },
      formats: ['es'],
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'react-router-dom', '@tanstack/react-query'],
      output: {
        entryFileNames: '[name].js',
      },
    },
    outDir: 'dist',
  },
})
