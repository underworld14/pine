import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  // svelteTesting() is inert outside test mode; in test it resolves the browser
  // condition and registers @testing-library/svelte auto-cleanup.
  plugins: [tailwindcss(), sveltekit(), svelteTesting()],
  server: {
    proxy: {
      '/api': { target: 'http://localhost:3412', ws: false },
      '/attachments': 'http://localhost:3412'
    }
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.{test,spec}.{js,ts}']
  }
});
