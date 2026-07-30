# Week-one push drill — real devices, real decisions

**Status:** ready to run at first deploy. **Not run yet.**
**Owner:** the operator. **Discharges:** TBD-OPERATOR(week-one real-device drill), Spec S15.11; G3 Def.6.
**Written:** 2026-07-30 (P3-B6-9). Platform facts below carry their own verification dates — **re-verify anything you are about to rely on before you rely on it.**

---

## 0. What this drill is, in the spec's own words

> **Week-one push drill:** TBD-OPERATOR(week-one real-device drill on household phones — iOS Home-Screen install + declarative push + `navigate` deep-link; Android push; the tap-with-tailnet-down edge; acceptance = notified → glance → decide < 10 s) [G3 Def.6].
>
> — `Spec/drafts/S15-frontend-api.md` §S15.11

And the loop target it is measuring:

> The loop target is normative: **notified → glance → decide < 10 s on a phone, while the host is up** (S3.7; Operating reality).

**This is a ceremony on phones, not a shell session.** There are no commands in it. Everything below is done on a device, with a stopwatch, by a person.

### The flip condition, verbatim

> **Flip condition [R17 §4.10]:** drill failure on household iPhones → notifications move to ntfy-with-relay; Web Push stays for badges only.
>
> — `Spec/drafts/S15-frontend-api.md` §S15.11

So this drill has a real decision hanging off it. If §4 records a FAIL on the household iPhones, the notification channel changes — that is a deliberate, pre-ratified fork, not a bug to fix under pressure.

---

## 1. Before you start

1. The control plane is running and reachable at `https://sinet.<your-tailnet>.ts.net` from the phone you are about to use. Open it in the phone's browser and sign in once. **Expected:** the workspace loads and the connection indicator says it is live.
2. There is at least one **real, answerable decision waiting** for the identity you will drill with. Open `/inbox` on the phone and confirm you can see one. **Expected:** at least one card you can answer.
   - If the inbox is empty, do not manufacture one by hand. Start any small piece of work and let the pipeline raise its own card — the drill is about a decision a person actually has to make.
3. Have a stopwatch. A phone's own stopwatch on a *second* device is easiest, because the drill phone will be busy.
4. Decide who each device belongs to and write it in §4's table before you begin, so nothing is attributed after the fact.

---

## 2. Per device class

Do the class that matches each household device. **Every step is one action and one expected result.** If a step's expected result does not happen, stop, write what happened in §4, and move to the next device — a partial pass is a finding, not a failure to hide.

### 2A. iPhone or iPad (iOS/iPadOS)

1. Open Safari on the device and go to `https://sinet.<your-tailnet>.ts.net`.
   **Expected:** the workspace loads.
2. Tap the **Share** button, scroll down, tap **Add to Home Screen**, then tap **Add**.
   **Expected:** a Sinet icon appears on the Home Screen. It should be the dark rounded square with a bright hub and three satellites.
3. Close Safari completely. Open Sinet **from the Home Screen icon**.
   **Expected:** it opens without Safari's address bar. *(Home Screen installation is still a hard prerequisite for any web push on iOS — see §5.6.)*
4. Sign in if asked.
5. Go to **Settings** and find **Notifications on this device**.
   **Expected:** it offers to enrol this device. If instead it says the browser does not offer web push, write down exactly what it says and go to §4 — that is the finding.
6. Type a name for the device (e.g. "Alice's iPhone") and tap **Enrol this device**.
   **Expected:** iOS asks whether to allow notifications. Tap **Allow**.
7. **Expected:** the panel now says the device is enrolled, and the device appears in the list below with the name you gave it.
8. **Lock the phone and put it down.** Start the stopwatch when the notification arrives, not before.
9. Wait for a decision to reach you. (An approval-class card is pushed once it has been waiting 24 hours, and again every 24 hours; a safety escalation is pushed immediately. If you would rather not wait a day, use a safety-class escalation, or lower `verification.card_push_hours` in Settings for the drill and put it back afterwards — **write down that you did**, because it changes what the drill measured.)
   **Expected:** a notification appears on the lock screen saying a decision is waiting.
10. **Start the stopwatch. Tap the notification.**
    **Expected:** Sinet opens **directly on that decision** — not the inbox list, not the home surface. The card on screen is the one the notification was about.
11. Read the card and answer it.
    **Expected:** the answer is accepted and the card leaves the queue.
12. **Stop the stopwatch** the moment the answer is accepted. Write the time in §4.
13. Look at the Sinet icon on the Home Screen.
    **Expected:** the badge count matches the number of items your inbox now shows. If badges are off for Sinet in iOS Settings → Notifications, note that instead of recording a mismatch.

### 2B. Android phone or tablet

1. Open Chrome and go to `https://sinet.<your-tailnet>.ts.net`.
   **Expected:** the workspace loads.
2. Open Chrome's menu and choose **Add to Home screen** / **Install app**.
   **Expected:** a Sinet icon appears.
   *(Installation is not required for push on Android — it is done here so the drill measures the same thing on both platforms.)*
3. Open Sinet from the icon and sign in if asked.
4. Go to **Settings** → **Notifications on this device**, name the device, tap **Enrol this device**.
   **Expected:** Android asks whether to allow notifications. Tap **Allow**. The panel then says the device is enrolled.
5. Steps 8–13 of §2A, unchanged.
   **Expected at step 13:** the badge appears on the app icon if the launcher supports it. Many Android launchers show a dot rather than a number — record which you saw.

### 2C. Mac (Safari)

1. Open Safari and go to `https://sinet.<your-tailnet>.ts.net`.
2. Go to **Settings** → **Notifications on this device**, name it, click **Enrol this device**, allow the prompt.
   **Expected:** the panel says the device is enrolled.
3. Steps 8–12 of §2A.
   **Expected:** the notification appears in Notification Centre and clicking it opens the decision.

### 2D. The tailnet-down edge — do this LAST, on one device that passed

This is the edge S15.11 names by itself, and it is the one most likely to be surprising.

1. With the device enrolled and a decision waiting, **turn Tailscale off on the phone** (or turn on airplane mode).
2. Wait for a push to arrive. **Expected:** the notification STILL ARRIVES. It travels through Apple's or Google's push service, not through your tailnet, so the host being unreachable does not stop it.
3. **Tap it.**
   **Expected:** Sinet opens and **says plainly that it cannot reach the control plane** — "the host may be asleep or the tailnet down" or the browser's own error page if the app was fully closed. **It must not show a stale copy of the inbox as though it were current.**
4. Write down exactly what you saw: which of the two it was, and whether anything on screen could have been mistaken for live data.
5. Turn Tailscale back on and reload.
   **Expected:** the decision is there and answerable.

> **Why the app may show the browser's own error page rather than a Sinet screen:** the service worker Sinet installs handles push and nothing else — it does not intercept page loads, so there is no cached shell to serve. That is deliberate: a cached shell is how an app ends up showing yesterday's data as though it were today's. A cold launch with the host unreachable is honestly an error, and the browser says so.

---

## 3. Acceptance

The drill PASSES on a device when all of these held:

- [ ] the device enrolled without an unexplained failure;
- [ ] a notification arrived for a real waiting decision;
- [ ] tapping it landed on **that decision**, not somewhere near it;
- [ ] **notified → glance → decide took under 10 seconds** (§2A step 12);
- [ ] the badge agreed with what the inbox showed, or badges were off and that was why;
- [ ] the tailnet-down edge (§2D) showed an honest unreachable state and never stale data posing as live.

---

## 4. Recorded results

Fill this in as you go. An empty cell is a result too.

| # | Device | Class (2A/2B/2C) | Owner | Enrolled? | Notification arrived? | Landed on the right card? | Seconds (notified → decided) | Badge matched? | Notes |
|---|--------|------------------|-------|-----------|----------------------|---------------------------|------------------------------|----------------|-------|
| 1 |        |                  |       |           |                      |                           |                              |                |       |
| 2 |        |                  |       |           |                      |                           |                              |                |       |
| 3 |        |                  |       |           |                      |                           |                              |                |       |
| 4 |        |                  |       |           |                      |                           |                              |                |       |

**Tailnet-down edge (§2D), on device #___:**

- Notification still arrived: ☐ yes ☐ no
- What the app showed on tap: _______________________________________________
- Could anything on screen have been read as live data? ☐ no ☐ yes — what: ____

**Did you change `verification.card_push_hours` for the drill?** ☐ no ☐ yes — from ___ to ___, put back on ___

**Date run:** ____________  **Run by:** ____________

### The decision this record drives

- **All household iPhones passed** → Web Push stands as the notification channel. Record the pass and close TBD-OPERATOR(week-one real-device drill).
- **The household iPhones FAILED** → the ratified flip condition applies: *"notifications move to ntfy-with-relay; Web Push stays for badges only."* That is a v1 work item, not a patch to make here. Write down which step failed and what the device said — the flip is a decision, and it deserves the evidence.

---

## 5. The platform facts this drill rests on

Each was verified on the date shown. **Re-verify any you are about to depend on** — this is a live-facts area and the platforms move.

1. **Declarative Web Push is shipped on Apple platforms only.** iOS/iPadOS 18.4 and macOS Safari 18.5. Verified 2026-07-30 against `https://webkit.org/blog/16535/meet-declarative-web-push/` (2025-03-27) and the Safari 18.5 release-notes post `https://webkit.org/blog/16923/`; the later WebKit posts through Safari 26.6 (`https://webkit.org/blog/18178/`, 2026-07-27) and the Safari 27 beta post mention no push change.
2. **Chromium and Firefox do NOT have it.** Verified 2026-07-30: the chromestatus API returns no such feature; Firefox bugs `1935325` and `1946085` are both status **NEW** (Bugzilla REST, last touched 2025-09/10) with a positive Mozilla standards position. **This is why Sinet installs a service worker** — every non-Apple device needs the classic subscription, which structurally requires one.
3. **The payload contract is W3C Push API §3.3** (Working Draft, 2025-12-01, `https://www.w3.org/TR/push-api/`, checked 2026-07-30): `web_push: 8030` and `notification` are required; `notification.title` and `notification.navigate` are required.
4. **`app_badge` is a WebKit EXTENSION, not standard.** It appears nowhere in the W3C draft (full text checked 2026-07-30); WebKit's own example writes it as a string inside `notification`. On non-Apple devices Sinet sets the badge through the standard Badging API from the same field instead.
5. **Apple's sender rules** (`https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers`, checked 2026-07-30): a `TTL` header is required and must be positive; `Urgency` is one of `very-low|low|normal|high`; the payload cap is 4 KB; a JWT may not expire more than **one day** ahead and should not be refreshed more than **once per hour**; and **"You don't need to join the Apple Developer Program to send web push notifications."**
6. **Home Screen installation is still required for any web push on iOS**, including on iOS 26. Verified 2026-07-30. Classic web push needs iOS 16.4+; the declarative form needs 18.4+. One iOS 26 change worth knowing: every site added to the Home Screen now opens as a web app by default, and a person can turn that off per site — if somebody does, push stops working for that site.
7. **Safari revokes push permission for a site that pushes invisibly.** Sinet therefore always shows a notification; there is no silent badge-only push.

## 6. What the notification can and cannot tell you

Worth knowing before the drill so nothing reads as a defect:

- The notification says **that** a decision is waiting and carries the deep link and the count. It does **not** say what the decision is. That is deliberate: the vendor relay sees timing and volume, and the platform's answer to that is to put nothing in the title worth seeing on a lock screen. The glance happens in Sinet, which is what the <10 s target measures.
- **What leaves this machine, exactly** (accepted-external-observables register row 2, Spec S01.8): push timing, volume and endpoint metadata reach Apple's or Google's push service. The notification content is encrypted to the device's own keys before it leaves, so the relay sees *when* and *how often* — never *what*. Sinet's enrolment panel says this on screen, in the server's own words.
- A push that could not be delivered before the next one is due is dropped rather than queued, so a phone that was off for a day gets the current nag, not a stack of old ones.
- If a device stops receiving notifications and the platform's own list still shows it, the push service has probably retired the subscription. Enrol it again — it is safe to repeat, and the platform replaces the row rather than adding a second device.

## 7. Known limits, so they are not discovered as surprises

- **A failed send waits a full cadence before the next attempt.** If a push service is briefly unreachable, the platform records the failure and tries again at the next due time rather than retrying immediately — a retry loop against a service that is refusing is how a sender earns a rate-limit ban. For a safety escalation that is the re-ping interval; for an approval card it is `verification.card_push_hours`.
- **Rotating the VAPID key invalidates every subscription in the household.** There is no automatic rotation for exactly that reason. If it ever has to happen, every device must be enrolled again, and this drill is what proves they were.
- **The platform never wakes a sleeping host.** Availability is best-effort while the host is up (Spec S01.7). A decision raised while the host is asleep is pushed when it wakes, not before.
