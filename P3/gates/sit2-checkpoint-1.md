# P3-SIT-2 checkpoint 1: approve the product map for the code review page — gate record (opened 2026-09-22)
Status: OPEN
answered: no

## What this decides

Packet P3-SIT-2 rebuilds the page where you review finished code ("code as the deliverable, frontend"). Before anything is built, FRONTEND.md asks you to approve the product map: `P3/design/product-map.md` §9 to §17 (map v4, commit `1861ef8`). Item 0 approves the map; items 1 to 4 are the map's own questions Q1 to Q4. Once approved, a fresh author builds from the map as canon, stopping next at checkpoint 2 (screenshots).

## The page after the packet, in plain words

After the packet the review page is one screen with a fixed reading order:

- **Top, the decision strip.** The task's title, whose work it is, "version N of M", the state in one sentence ("This finished work is waiting for you. Nobody else reviews it."), and the checker's one-line verdict, from the platform's own record. Buttons: **Accept this work**, **Ask for changes** (Q4), **Start a follow-up task**, **Try it**. A button the platform has closed shows its reason instead of sitting dead.
- **Left, the file list.** Every file: path, kind of change (new, changed, deleted, renamed from), size, lines added and removed, a "binary" mark where there is no text, a badge where a comment or finding sits in that file. A totals line ("23 files, all new · +1,301 lines"). On a phone the list becomes a picker above the reading pane.
- **Right, the reading pane.** One file at a time, with a toggle: **Changes** (only the lines that changed, side by side on desktop, stacked on a phone) or **Whole file** (the full text at this version, with line numbers). A new file opens as Whole file, a changed file as Changes (Q3). A binary file shows its sizes and says there is no text. A file cut short says so, with the platform's reason. Click a line's margin to comment on that line.
- **Below the pane, what changed since last time.** A version strip (v1, v2, newest selected) and "compared with: the previous version", with a picker: the previous version, the project before the task, or any version. Version 1 says plainly it is compared with the project before the task, so everything in it is new. List and pane follow the pair you pick.
- **What the worker says it did.** The written report, folded under a plain heading, led by: "This is the worker's own account of the work. It is a claim, not a check: the files above are the fact and the checker's findings are below." Open by default only when the work is a document rather than code.
- **What the checker found, and your comments.** The bootstrap notice when it applies (no build or test commands captured, so your review is what decides), then every finding and comment of this version with its category, severity and a jump into the file. Comment on the whole work or on one line (what a comment does at v0: item 4).
- **Try it.** Before any click: "Try it live: not available on this platform yet", the gap is the platform's, not the work's, and it does not block accepting; then the host command that runs this exact version beside the platform, read-only (Q2). **Launch** stays and shows the platform's answer word for word.
- **Accept.** Same card and flow as today, reached from the strip.
- **The record, folded.** Every revision with its pin, run and time; every door with its technical detail; downloads.

Two riders: the task's card groups its run story per attempt (a crash and the attempt that picked the work up read as one story), and a receipt that carries a judge shows "Checked by «model»", with a yellow "same model family" chip when checker and worker share a family.

What stays honest when something is absent: nothing on the page is invented. Every count, size, kind and pin is a field the platform served. If the file list could not be built, the page says so, with the reason. If the platform can no longer find the saved version, the page says that is a platform fault, not the work's. A missing file, a cut, a closed door: each shows the platform's own words.

## What you see today (the before-frames)

- `P3/design/rework-screens/sit2/before-deliverable-desktop-top.png`: the top of today's review page on a desktop: the notices, then "The work" opens with the worker's written report and not one file in sight.
- `P3/design/rework-screens/sit2/before-deliverable-desktop.png`: the same page at full length (about 35 screen-heights): after the report, all 23 files printed as one continuous green diff, revisions and doors at the very bottom.
- `P3/design/rework-screens/sit2/before-deliverable-phone.png`: the phone view: the same order in one narrow column, the report's table crushed, the code nowhere near the top.
- `P3/design/rework-screens/sit2/before-task-desktop.png`: the task's card ("Create a webshop for car replacement parts and tuning parts", marked DONE) opening with the confirmed specification; nothing on the first screen says finished work is waiting or where to open it.

## Looking at today's page yourself

The reference world `~/.sinet-sit2-builder` (port 8489) is stopped: nothing listens on 8489, and production on 8482 is not touched. The recorded start command is:

    SINET_CLICKTHROUGH_STATE=$HOME/.sinet-sit2-builder SINET_CLICKTHROUGH_PORT=8489 ./P3/gates/B6-clickthrough.sh

The record adds that this script stops at the local-tier wiring, so the builder ran the door-built binary directly (that exact command is not recorded), and that sign-in is "as the seed printed" at start: no ID or PIN is recorded here and none is invented. Once up, the frames' page is `/deliverables/dlv-t-3120e8e3d14591d3`. The four frames are enough to decide this checkpoint.

## Items

### 0. Approve the map (`P3/design/product-map.md` §9 to §17)

- Options: **approve** / **approve with changes** / **redo**.
- Effect: approve: a fresh author builds from this map as canon, step 1 first (the page shell on the real webshop), and stops at checkpoint 2 with screenshots. Approve with changes: write them under Answers; the map gets a dated edit and the build starts on everything else. Redo: the map goes back with your reasons; nothing is built until the next version is approved.
- Recommendation: **approve**. It carries the review layout you ratified onto a full page and keeps every behaviour that already works.

### 1. Q1 The Reviews index at `/reviews` as the front door: build it in this packet (recommended, small) or keep the placeholder?

- Options: **yes, build it** / **keep the placeholder**.
- Effect: yes: Reviews in the sidebar becomes a list of every piece of work you may open, grouped "Waiting for you", "Waiting for someone else", "Accepted", "Superseded", one row each (task title, kind, version, whose, when it last moved, Open review); small, built on two reads the app already makes. Keep: Reviews stays the placeholder; you reach review pages only from Home's "What needs me", the task's card, or a notification link.
- Recommendation: **yes, build it**. It is the direct answer to "no idea where I have to click".

### 2. Q2 The "try it now" script line (11.4): on the page for everyone, or operator-only?

- Options: **everyone** / **operator only**.
- Effect: everyone: each household member sees the one-line host command with the exact deliverable id under the "not available yet" notice; it only does anything for someone who can already open a terminal on the host. Operator only: the household sees the notice alone, without the "what to do instead" line.
- Recommendation: **everyone**. One page for all, honest about the gap and what to do instead; the section goes away when P3-SIT-3 lands the live preview.

### 3. Q3 New files open as **Whole file** by default, changed files as **Changes**: agree?

- Options: **agree** / **different defaults** (say which).
- Effect: agree: a new file opens as its full text with line numbers (a wall of green additions is not how a person reads a new file); a changed file opens showing only what changed; one toggle flips either view. Different: state the rule you want and the author applies it.
- Recommendation: **agree**.

### 4. Q4 The word for the send-back action on the strip: "Ask for changes" (proposed) or another phrase you use?

- Options: **"Ask for changes"** / **your own phrase** (write it).
- Effect: the label of the top strip's send-back button. It jumps to the comment box; at v0 a comment is what the next round works from, and the real "request a revision" door opens only when the platform asks. On accepted work the button starts a follow-up instead, and the strip says so.
- Recommendation: **"Ask for changes"**. Plain, matches the "say what to change" wording already on the page, and promises no retry the platform does not have yet.

## Answers

Answer here (next to an item or below, dated) or in free text to any session; either is authoritative. A partial answer (set `answered: partial`) lets the build proceed on the approved parts while the rest waits.
