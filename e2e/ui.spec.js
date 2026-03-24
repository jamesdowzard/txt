const { test, expect } = require('@playwright/test');

const hikingDraft = 'Count me in for Saturday! Lands End trail looks clear — 62°F and sunny. Want me to bring snacks?';

async function openConversation(page, name) {
  await page.getByText(name, { exact: true }).click();
  await expect(page.locator('#chat-header-name')).toHaveText(name);
}

test.beforeEach(async ({ page, request }) => {
  await request.post('/_e2e/drafts', {
    data: {
      body: hikingDraft,
      conversation_id: 'conv3',
      draft_id: 'draft1',
    },
  });
  await page.goto('/');
});

test('loads the seeded conversation list and thread view', async ({ page }) => {
  await expect(page.locator('#connection-banner.disconnected')).toHaveCount(0);
  await expect(page.locator('#conversation-list .convo-item')).toHaveCount(9);
  await openConversation(page, 'Sarah Chen');
  await expect(page.locator('#messages-area')).toContainText('Hey! Are you free for dinner tonight?');
  await expect(page.locator('#compose-input')).toBeVisible();
});

test('sends a message through the compose box', async ({ page }) => {
  const outbound = `Playwright outbound ${Date.now()}`;

  await openConversation(page, 'Sarah Chen');
  await page.locator('#compose-input').fill(outbound);
  await page.locator('#send-btn').click();

  await expect(page.locator('#messages-area')).toContainText(outbound);
});

test('refreshes the active thread from SSE invalidations', async ({ page, request }) => {
  const inbound = `SSE inbound ${Date.now()}`;

  await openConversation(page, 'Sarah Chen');
  await request.post('/_e2e/messages', {
    data: {
      body: inbound,
      conversation_id: 'conv1',
      sender_name: 'Sarah Chen',
      sender_number: '+14155551234',
    },
  });

  await expect(page.locator('#messages-area')).toContainText(inbound);
});

test('loads older pages when scrolling up in a long thread', async ({ page }) => {
  await openConversation(page, 'Paged Thread');
  await expect(page.locator('#messages-area .msg')).toHaveCount(100);
  await expect(page.locator('#messages-area')).not.toContainText('Paged message 001');

  await page.locator('#messages-area').evaluate((el) => {
    el.scrollTop = 0;
    el.dispatchEvent(new Event('scroll'));
  });

  await expect(page.locator('#messages-area')).toContainText('Paged message 001');
  await expect(page.locator('#messages-area .msg')).toHaveCount(150);
});

test('sends an AI draft from the thread banner', async ({ page }) => {
  const draftReply = `Draft sent ${Date.now()}`;

  await openConversation(page, 'Weekend Hiking Group');
  await expect(page.locator('.draft-banner')).toHaveCount(1);
  await page.locator('.draft-text').fill(draftReply);
  await page.locator('.draft-send-btn').click();

  await expect(page.locator('.draft-banner')).toHaveCount(0);
  await expect(page.locator('#messages-area')).toContainText(draftReply);
});

test('discards an AI draft from the thread banner', async ({ page }) => {
  await openConversation(page, 'Weekend Hiking Group');
  await expect(page.locator('.draft-banner')).toHaveCount(1);
  await page.locator('.draft-discard-btn').click();
  await expect(page.locator('.draft-banner')).toHaveCount(0);
});
