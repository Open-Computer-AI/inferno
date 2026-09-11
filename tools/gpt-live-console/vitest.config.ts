import { defineConfig } from 'vitest/config';
import path from 'node:path';
export default defineConfig({ root: path.dirname(new URL(import.meta.url).pathname), test: { environment: 'jsdom', globals: true } });
