// Main path against the composed stack (run via e2e/run.sh, which seeds the data and sets the env).
import { test, expect } from '@playwright/test';
import { execSync } from 'node:child_process';

const mailpit = `http://localhost:${process.env.MAILPIT_PORT}`;
const compose = process.env.E2E_COMPOSE;
const email = `resident-${Date.now()}@example.test`;

// The newest unseen message to `email` whose body links to `/<page>?token=`; returns the path and token query.
const seen = new Set();
async function linkFromMail(page) {
  const re = new RegExp(`/${page}\\?token=[\\w.~%-]+`);
  for (let i = 0; i < 60; i++) {
    const { messages } = await (await fetch(`${mailpit}/api/v1/messages`)).json();
    for (const m of messages) {
      if (seen.has(m.ID) || !m.To.some((t) => t.Address === email)) continue;
      const { Text } = await (await fetch(`${mailpit}/api/v1/message/${m.ID}`)).json();
      const hit = Text.match(re);
      if (hit) { seen.add(m.ID); return hit[0]; }
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`no ${page} mail for ${email}`);
}

async function pickAddress(page, number) {
  await page.getByLabel('Settlement').selectOption({ label: 'Варна' });
  await page.getByLabel('Street').fill('Ferd');
  await page.getByRole('button', { name: 'улица Фердинанд' }).click();
  await page.getByLabel('Number').fill(number);
  await page.getByRole('button', { name: 'Find' }).click();
  await page.getByRole('button', { name: 'Confirm position' }).click();
}

test('region page to unsubscribe', async ({ page }) => {
  await page.route('**/turnstile/v0/api.js*', (r) =>
    r.fulfill({ contentType: 'text/javascript', body: `window.turnstile={render:(el,o)=>{setTimeout(()=>o.callback('e2e-token'),0)},reset(){}}` }));
  await page.route('**/tile.openstreetmap.org/**', (r) => r.abort());

  // 1. Region page, Bulgarian then English
  await page.goto('/');
  await page.getByRole('combobox').first().selectOption({ label: 'Варна' });
  await expect(page.getByTestId('status-strip')).toContainText('данните са актуални');
  await expect(page.locator('li[id^=outage-]')).toContainText('аварийно');
  await page.getByRole('button', { name: 'EN' }).click();
  await expect(page.getByRole('heading', { name: 'Outages in your area' })).toBeVisible();
  await expect(page.locator('li[id^=outage-]')).toContainText('emergency');

  // 2. Address check: number 10 is on the Outage, 12 is not
  await pickAddress(page, '12');
  await expect(page.getByTestId('check')).toContainText('water: Not affected');
  await page.getByLabel('Number').fill('10');
  await page.getByRole('button', { name: 'Find' }).click();
  await page.getByRole('button', { name: 'Confirm position' }).click();
  await expect(page.getByTestId('check')).toContainText('water: Affected');

  // 3. Subscribe (Get alerts keeps the Address), in English
  await page.getByRole('link', { name: 'Get alerts' }).click();
  await page.getByRole('button', { name: 'EN' }).click();
  await expect(page.getByTestId('addr')).toContainText('10');
  await page.getByLabel('Email').fill(email);
  await page.getByRole('button', { name: 'Subscribe' }).click();
  await expect(page.getByTestId('result')).toContainText('verification email is on its way');

  // 4. Verify from the Mailpit email
  await page.goto(await linkFromMail('verify'));
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: 'Потвърди' }).click();
  await expect(page.getByTestId('result')).toContainText('Абонаментът е потвърден');

  // 5. Manage: request a link, add a second Watched Address
  await page.goto('/manage');
  await page.getByLabel('Имейл').fill(email);
  await page.getByRole('button', { name: 'Изпрати връзка' }).click();
  await page.goto(await linkFromMail('manage'));
  await expect(page.getByRole('heading', { name: 'Manage your subscription' })).toBeVisible();
  await pickAddress(page, '12');
  await expect(page.getByRole('listitem').filter({ hasText: /12 Remove/ })).toBeVisible();

  // 6. Provider goes live through the CLI; the Outage's scan sends an Alert
  execSync(`${compose} run --rm app provider-mode "ВиК Варна" live`, { stdio: 'inherit' });
  execSync(`${compose} exec -T postgres psql -U postgres -d outage -c "INSERT INTO jobs (kind, payload) SELECT 'scan-alerts', id::text::bytea FROM notices WHERE external_id = 'e2e-1'"`, { stdio: 'inherit' });

  // 7. Unsubscribe with the real token from the Alert email
  await page.goto(await linkFromMail('unsubscribe'));
  await page.getByRole('button', { name: 'EN' }).click();
  await page.getByRole('button', { name: 'Unsubscribe' }).click();
  await expect(page.getByTestId('result')).toContainText('You will no longer get alerts');
});
