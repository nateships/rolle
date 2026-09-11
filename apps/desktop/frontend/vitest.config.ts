import path from "node:path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Test config on its own so the Wails and Tailwind plugins stay out of the test run.
export default defineConfig({
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "./src") },
  },
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    css: false,
  },
});
