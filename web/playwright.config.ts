import{defineConfig}from"@playwright/test";
export default defineConfig({testDir:"./e2e",timeout:30_000,use:{baseURL:process.env.E2E_BASE_URL??"http://127.0.0.1:3000",trace:"retain-on-failure",launchOptions:process.env.PLAYWRIGHT_CHROME_PATH?{executablePath:process.env.PLAYWRIGHT_CHROME_PATH}:undefined},reporter:"line",workers:1});
