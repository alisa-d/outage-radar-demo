export default {
  testMatch: 'main-path.spec.js',
  timeout: 90_000,
  workers: 1,
  use: { baseURL: `http://localhost:${process.env.FRONTEND_PORT ?? 8081}`, browserName: 'chromium' }
};
