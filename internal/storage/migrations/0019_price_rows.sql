-- 0019_price_rows.sql — the durable home of the S10.3 price table, the first of
-- the three S18.3 data-valued settings surfaces to get one.
--
-- Exact DDL per the S02.2 schema-workshop mandate; applied in ONE transaction
-- with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1). A committed migration is immutable
-- (CONVENTIONS §6) — 0001–0018 stay byte-untouched, this file included once it
-- lands.
--
-- ONE thing lands, with its reason (P3-B6-3A, OQ1(a)):
--
--   price_rows — the effective-dated per-{model, lane} rows of S10.3, stored.
--     Until now the price table was a SEAM with no storage: EffectiveDatedTable
--     carried the S10.3 semantics in memory and the shell composed it EMPTY, so
--     the operator surface S15.9 promises ("the API-equivalent price table" is
--     present and editable) had nothing to read or write. S01.10 says what shape
--     the storage takes: "The D5 price table is its own owner-attributed table
--     under the same audit pattern" — its own table, owner-attributed, audited
--     like a registry write.
--
-- APPEND-ONLY, AND THAT IS THE EDIT MODEL, NOT A RESTRICTION ON IT.
-- Effective-dating means a price CHANGE is a new row with a later
-- `effective_from`, never an edit of the row that priced the past: receipts
-- already priced against an old row must stay reproducible, and a table that
-- let its history be rewritten could not answer "what did this cost when we
-- charged it?" (S10.1 stamps `table_version` on every priced cost for exactly
-- that reason). So there is no UPDATE verb to guard — the triggers below make
-- the absence structural rather than a convention, the way 0007 does for review
-- comments and 0012 for revalidation stamps.
--
-- THE TABLE SHIPS EMPTY, and that is the ratified posture, not an oversight.
-- An empty table prices every row UNPRICED, never $0 (S10.1; P-T08-1;
-- CONVENTIONS §11) — the B1-gate posture stands unchanged. Seeding it from the
-- vendored genai-prices data is a separate OPERATOR act (G2 Def.16: a refresh
-- lands as a proposal, never an overwrite), which this surface now makes
-- possible; no migration seeds a price, because a price nobody ratified is a
-- number the platform would then bill against.
--
-- NO MONEY IS COMPUTED IN THIS FILE. `unit_prices` is stored as opaque JSON and
-- is only ever read back out whole: there is no view, no expression and no
-- generated column that multiplies, sums or converts a price here. Pricing is
-- internal/metering's, per the D5/§11 read-model rule.

CREATE TABLE price_rows (
    price_row_id   INTEGER PRIMARY KEY,
    -- The S10.3 row identity: {model, lane} priced as of a date.
    model          TEXT NOT NULL CHECK (model <> ''),
    lane           TEXT NOT NULL CHECK (lane <> ''),
    -- unit_prices is the JSON object of per-SINGLE-token dollar rates
    -- {input_usd, output_usd, cache_read_usd, cache_creation_usd,
    -- server_tool_usd} — per token rather than per million so the arithmetic
    -- metering does with them is exact (internal/metering/price.go).
    unit_prices    TEXT NOT NULL,
    -- effective_from is first-class: future-dated rows are admitted and simply
    -- do not apply until their date, because a table without effective dates is
    -- guaranteed wrong on a known future price flip (S10.3; P-T08-3).
    effective_from TEXT NOT NULL,
    -- verified_on + source are the provenance S10.3 requires: which day this
    -- was checked against the provider, and where it came from. The D5 rule
    -- that the table quotes the provider actually serving the lane — never an
    -- aggregator's cheapest host — lives in what an operator puts here.
    verified_on    TEXT NOT NULL,
    source         TEXT NOT NULL CHECK (source <> ''),
    -- created_by/created_ts are the owner attribution S01.10 asks for: the
    -- table IS its own audit record, so who added a row and when are columns of
    -- the row, not a separate trail that could go missing.
    created_by     TEXT NOT NULL CHECK (created_by <> ''),
    created_ts     TEXT NOT NULL,
    reason         TEXT NOT NULL DEFAULT ''
);

-- The lookup the pricing path performs: the latest row for {model, lane} whose
-- effective_from is not after the usage timestamp (S10.3 semantics).
CREATE INDEX price_rows_lookup_idx ON price_rows (model, lane, effective_from);

CREATE TRIGGER price_rows_immutable
    BEFORE UPDATE ON price_rows
BEGIN
    SELECT RAISE(ABORT, 'a price row is immutable: a price change is a new effective-dated row, so the price a receipt was charged at stays reproducible (Spec S10.3)');
END;

CREATE TRIGGER price_rows_no_delete
    BEFORE DELETE ON price_rows
BEGIN
    SELECT RAISE(ABORT, 'price rows are never deleted: the table is the audit record of what was priced when (Spec S01.10; S10.3)');
END;
