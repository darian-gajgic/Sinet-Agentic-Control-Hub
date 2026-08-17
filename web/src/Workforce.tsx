import { api, type RoutedRun, type WorkerRow, type WorkerVersion, type Workforce as WorkforceData } from './api'
import type { EventStream } from './events'
import { useLive, workforceEventTypes } from './live'
import { Absent, Freshness, Money, Owner, Section, Stamp, SurfaceHead } from './parts'
import { Chip as ToneChip, EmptyState } from './ui'

/**
 * The workforce map (Spec S15.10; S08.1/S08.2/S08.4/S08.7/S08.8/S08.9).
 *
 * A readable view of the machinery: every worker the caller may see, what each
 * is equipped with, how its multi-stage procedures connect, and the per-version
 * quality and cost readings beside it. Its purpose is audit and understanding.
 *
 * THREE THINGS THIS SURFACE IS BUILT AROUND.
 *
 * IT IS VIEW-ONLY, STRUCTURALLY. Editing through the map is parked to 15.5, so
 * there is no form, no button that posts, and no verb in `api` to reach: the
 * whole module makes exactly one GET. A DOM walk and a source scan both assert
 * it, because "I read the code and it looked fine" is not a check.
 *
 * NOTHING HERE IS INVENTED, AND THE ABSENCES SAY WHY. A worker with no active
 * version has no equipment to show; a version nobody approved has no permissions
 * block; a version nothing was ever routed to has no outcomes. At v0 the roster
 * is EMPTY on a fresh host — machinery is composed when recurring work earns it
 * (S08.6) — and that empty is the honest answer, not a blank page. Every one of
 * those states renders with the server's own reason.
 *
 * THE RENDERING IS OWNED (TBD-P3(workforce-map rendering approach), RESOLVED
 * 2026-07-29 — see P3/CONVENTIONS.md §45-B). Semantic HTML and CSS grouped by
 * domain and kind, in the §42 view idiom. No graph-layout library entered: the
 * roster is household-scale and empty at v0, the surface is view-only with no
 * interaction physics, and a layout dependency would be a new adoption needing
 * the full S16.4 rail plus gate ratification.
 */
export function Workforce({ stream }: { stream?: EventStream }) {
  const { data, error, stale } = useLive({
    key: '/api/workforce',
    read: () => api.workforce(),
    types: workforceEventTypes,
    stream,
  })

  return (
    <section className="surface workforce">
      {/* VIEW-ONLY is stated, not merely enforced: the map offers no act at all
          at v0 (S15.10 parks editing to 15.5), and a reader who cannot find the
          edit button deserves to be told there is none rather than left
          hunting. */}
      <SurfaceHead
        title="Workforce"
        what="The machinery: every worker you may see, what each is equipped with, how its steps connect, and the outcomes recorded against each version. View-only at v0 — nothing here can be changed from this screen."
      />
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {data !== null && <Roster data={data} stale={stale} />}
    </section>
  )
}

function Roster({ data, stale }: { data: WorkforceData; stale: boolean }) {
  const groups = byDomain(data.workers)
  return (
    <>
      {/* The two scopes are SERVED sentences. A member's roster and the
          operator's are different readings of the registry, and a surface that
          did not say which one it was showing would let one pose as the other. */}
      <p className="muted scope text-xs" data-roster-scope>
        Showing: {data.roster_scope}.
      </p>
      <p className="muted scope text-xs" data-outcome-scope>
        Run figures below cover: {data.outcome_scope}.
      </p>
      {data.truncated && (
        <p className="muted" data-truncated>
          More workers exist than this reading returned.
        </p>
      )}
      {data.outcomes_truncated && (
        <p className="muted" data-outcomes-truncated>
          The routing scan hit its bound, so a version showing no runs may simply have none within it.
        </p>
      )}

      {data.workers.length === 0 ? (
        <Section title="No workers yet" stale={stale}>
          {/* The S08.6/14.4 no-standing-army state, moved onto the teaching
              primitive with its sentence KEPT — it was already the best empty
              copy in the tree. No `action`: this surface offers none, and an
              empty state is the last place to invent one (§45-B). */}
          <EmptyState
            what="Nothing has been composed yet."
            why="Sinet does not pre-staff a formation: a worker is built when recurring work earns one, and until then the coordinator does the work with knowledge injected for the task."
          />
        </Section>
      ) : (
        groups.map(([domain, workers]) => (
          <Section key={domain} title={`Domain: ${domain}`} stale={stale}>
            <DomainMarking workers={workers} />
            <div className="worker-grid">
              {workers.map((w) => (
                <WorkerCard key={w.template_id} worker={w} />
              ))}
            </div>
          </Section>
        ))
      )}
    </>
  )
}

/** The domain's verification maturity. It is a STRUCTURAL fact, not a badge
 *  (S08.7): a degraded domain cannot deliver without a person looking, and the
 *  consequence rides every card below.
 *
 *  Read off the first worker in the group, which is sound because the group IS
 *  the domain: every card here resolved the same `domains` row, so they cannot
 *  disagree about its maturity. */
function DomainMarking({ workers }: { workers: WorkerRow[] }) {
  const d = workers[0].domain
  return (
    <p className="domain-marking text-xs" data-maturity={d.maturity}>
      Verification maturity: <strong>{d.maturity === '' ? <Absent reason="not recorded" /> : d.maturity}</strong>
      {d.maturity === 'degraded' && (
        <>
          {' '}
          — no purpose-built quality check exists for this domain yet, so nothing here delivers without a person
          reviewing it, <strong data-degraded-schedule>nothing here may attach to a schedule whose results
          auto-accept</strong>, and nothing here can graduate to unsupervised work.
        </>
      )}
      {d.rubric_ref !== undefined && d.rubric_ref !== '' && <> Rubric: {d.rubric_ref}.</>}
    </p>
  )
}

function WorkerCard({ worker: w }: { worker: WorkerRow }) {
  const active = w.versions.find((v) => v.active) ?? null
  const superseded = w.versions.filter((v) => !v.active)
  return (
    <article
      className="worker rounded-(--radius) border border-border bg-(image:--panel-grad) p-(--density-pad)"
      data-worker={w.template_id}
      data-kind={w.kind}
      data-scope={w.scope}
    >
      <header>
        <h3>{w.name}</h3>
        <p className="worker-identity flex flex-wrap items-center gap-2 text-xs">
          <ToneChip data-worker-kind>{w.kind}</ToneChip> <ToneChip data-worker-scope>{w.scope}</ToneChip>{' '}
          <ToneChip data-worker-status tone={w.status === 'active' ? 'green' : 'accent'}>
            {w.status}
          </ToneChip>{' '}
          <Owner id={w.owner} />
        </p>
      </header>

      <Delivery worker={w} />

      {/* THE DEFINITION AND THE PERMISSIONS ARE SEPARATE RENDERS, and that is
          the audit case that matters most. A worker whose file failed its S08.3
          hash check has no readable definition — but it still HAS granted
          powers, recompiled into every invocation from the guardrails table
          (S08.2), and those are exactly what a person needs to see about a
          worker they have just been told they cannot trust. Gating the
          permissions block on the definition hid that half. */}
      {w.definition === null ? (
        <p className="absent" data-definition-absent>
          {w.definition_absent === undefined || w.definition_absent === '' ? (
            <Absent reason="no definition was read for this worker" />
          ) : (
            w.definition_absent
          )}
        </p>
      ) : (
        w.definition.description !== undefined &&
        w.definition.description !== '' && <p className="worker-description">{w.definition.description}</p>
      )}
      <Equipment worker={w} granted={active} />
      {w.definition !== null && <Connections worker={w} />}

      <div className="worker-versions">
        <h4>Versions</h4>
        {active !== null && <VersionBlock version={active} />}
        {superseded.map((v) => (
          // A superseded version is HISTORY, so it opens closed. What the worker
          // IS lives on the active version above; a wall of five permission
          // blocks would bury it (the one display choice on this surface, with
          // its reason in §45-B).
          <details key={v.version_id} className="version-history" data-superseded={v.version_id}>
            <summary>
              Version {v.version} — superseded{v.approved_ts === undefined || v.approved_ts === '' ? ' (never approved)' : ''}
            </summary>
            <VersionBlock version={v} />
          </details>
        ))}
        {w.versions_truncated && (
          <p className="muted" data-versions-truncated>
            Older versions exist than this reading returned.
          </p>
        )}
      </div>
    </article>
  )
}

/**
 * The S08.7 delivery policy, served from the registry's own read.
 *
 * There is no absence branch here and that is deliberate: the server fails the
 * whole read rather than serving an unread policy, so this can never be handed a
 * default — and the only default it could have picked, "needs no review", is the
 * one direction this must never fail in.
 */
function Delivery({ worker: w }: { worker: WorkerRow }) {
  const d = w.delivery
  return (
    <p className="worker-delivery" data-requires-review={String(d.requires_review)}>
      {d.deliverable
        ? d.requires_review
          ? 'Delivers only through a person’s review.'
          : 'Delivers without a review step.'
        : 'Does not deliver.'}
      {(d.reasons ?? []).length > 0 && (
        <ul className="delivery-reasons">
          {(d.reasons ?? []).map((r) => (
            <li key={r}>{r}</li>
          ))}
        </ul>
      )}
    </p>
  )
}

/**
 * Equipment, with the S08.2 split on screen (R12).
 *
 * The definition FILE may request equipment; approval copies requested → granted
 * only after the permission audit, and the granted set is the only thing that
 * enforces anything. Showing them as one list would erase the distinction the
 * whole guardrail split exists to make — so they are two blocks, labelled.
 *
 * "Helpers" is the fourth word in S15.10's list and it has no field to render
 * (OQ5, ratified): the S08.1 schema carries no helper slot, because helper
 * selection is per RUN through the same routing pipeline that picked this worker
 * (S08.8 step 5). So the map states that, and points at the place a helper spawn
 * really is visible — the routing cause on each routed run below. Rendering a
 * helper roster would mean inventing one.
 */
function Equipment({ worker: w, granted }: { worker: WorkerRow; granted: WorkerVersion | null }) {
  const req = w.definition?.requested_equipment ?? {}
  const lists: [string, string[] | undefined][] = [
    ['Tools', req.tools],
    ['Skills', req.skills],
    ['Knowledge selectors', req.knowledge],
    ['Connectors', req.connectors],
  ]
  return (
    <div className="worker-equipment">
      <h4>Equipment</h4>
      <div className="equipment-requested" data-equipment="requested">
        <h5>Requested by the definition</h5>
        {w.definition === null ? (
          // The requested half lives in the FILE, so it is the half that goes
          // with an unreadable definition. The granted half below does not.
          <Absent reason="the definition could not be read, so what it requested is unknown" />
        ) : (
          <dl>
            {lists.map(([label, items]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd>
                  {(items ?? []).length === 0 ? <Absent reason="none requested" /> : <Chips items={items ?? []} />}
                </dd>
              </div>
            ))}
          </dl>
        )}
      </div>

      <div className="equipment-granted" data-equipment="granted">
        <h5>Granted permissions</h5>
        {granted === null ? (
          <Absent reason="no active version, so nothing is granted" />
        ) : (
          <Grants version={granted} />
        )}
      </div>

      <p className="muted" data-helpers>
        Helpers: there is no standing helper roster to show. A helper is chosen per run, by the same routing pipeline
        that picked this worker, so what helped is recorded on the run — look for a <code>helper-spawn</code> routing
        cause among the runs below.
      </p>
    </div>
  )
}

/** The granted guardrails block, or the reason there is none. */
function Grants({ version }: { version: WorkerVersion }) {
  const g = version.granted
  const barred = g !== null && g.schedule_barred === true
  if (g === null) {
    return (
      <span className="absent" data-granted-absent>
        {version.granted_absent === undefined || version.granted_absent === '' ? (
          <Absent reason="no granted permissions were read" />
        ) : (
          version.granted_absent
        )}
      </span>
    )
  }
  return (
    <dl className="grants">
      <div>
        <dt>Tools</dt>
        <dd>{g.granted_tools.length === 0 ? <Absent reason="none granted" /> : <Chips items={g.granted_tools} />}</dd>
      </div>
      {(g.gated_tools ?? []).length > 0 && (
        <div>
          <dt>Gated tools</dt>
          <dd>
            <Chips items={g.gated_tools ?? []} />
          </dd>
        </div>
      )}
      <div>
        <dt>Confinement</dt>
        <dd data-confinement={g.confinement_class}>{g.confinement_class}</dd>
      </div>
      <div>
        <dt>Egress</dt>
        <dd data-egress={g.egress}>
          {g.egress}
          {(g.egress_hosts ?? []).length > 0 && (
            <>
              {' '}
              — <Chips items={g.egress_hosts ?? []} />
            </>
          )}
        </dd>
      </div>
      <div>
        <dt>Ceilings</dt>
        <dd data-budget={g.budget_usd === null ? 'absent' : 'declared'}>
          {g.budget_usd === null && g.budget_steps === null ? (
            // 0 is what the guardrails row stores for "none requested", and
            // approval grants only what was requested (S08.2) — so printing it
            // would be a ceiling nobody set (S10.1, the meters precedent).
            <Absent reason="no ceiling declared — this version requested none, and approval grants only what was requested" />
          ) : (
            <>
              {g.budget_usd === null ? (
                <Absent reason="no dollar ceiling declared" />
              ) : (
                <Money usd={g.budget_usd} />
              )}
              {' · '}
              {g.budget_steps === null ? (
                <Absent reason="no step ceiling declared" />
              ) : (
                <>{String(g.budget_steps)} steps</>
              )}
            </>
          )}
        </dd>
      </div>
      {g.permission_mode !== undefined && g.permission_mode !== '' && (
        <div>
          <dt>Permission mode</dt>
          <dd>{g.permission_mode}</dd>
        </div>
      )}
      {g.gate_policy !== undefined && g.gate_policy !== '' && (
        <div>
          <dt>Gate policy</dt>
          <dd>{g.gate_policy}</dd>
        </div>
      )}
      <div>
        <dt>Supervised first-N</dt>
        <dd data-first-n={String(g.first_n_remaining)}>
          {g.first_n_remaining > 0
            ? `${String(g.first_n_remaining)} more outputs need a person’s review before this graduates`
            : 'complete'}
        </dd>
      </div>
      <div>
        <dt>Attachable to a schedule</dt>
        {/* The guardrail is copied from the version's REQUEST unclamped, so it
            can say yes while the worker's domain bars it (S08.7). Rendering the
            grant alone would contradict the marking two blocks up; the bar is
            the operative fact and rides with it.

            THE BAR IS READ, NOT DERIVED. It used to be computed here from
            `maturity === 'degraded'` — a policy consequence decided in the
            browser, which is a side-truth the standing readings bar, and which
            leaves any future maturity that also bars schedules silently
            unmarked. It also had to be handed in by the caller, and the
            superseded-version block forgot to; served on the grants row, every
            render of a granted block carries its own. */}
        <dd data-schedule-attachable={String(g.schedule_attachable)} data-schedule-barred={String(barred)}>
          {g.schedule_attachable ? 'granted' : 'no'}
          {barred && (
            <> — but barred while this worker’s domain is degraded: a schedule that auto-accepts is closed to it (S08.7)</>
          )}
        </dd>
      </div>
    </dl>
  )
}

/**
 * How the multi-stage procedures connect (R13, OQ7 as ratified).
 *
 * At v0 that grounds to the registry's OWN multi-stage data. A kind=automation
 * worker carries a deterministic workflow: an ordered chain of steps, each
 * naming a connector verb, with the explicit approval nodes marked — an outward
 * step is never a direct call, it becomes a gated proposal (S08.9; D7). An
 * agentic worker has no chain; what connects it to work is its selectors and
 * its execution profile, so that is what renders instead.
 */
function Connections({ worker: w }: { worker: WorkerRow }) {
  const def = w.definition
  if (def === null) return null
  if (def.workflow !== undefined && def.workflow !== null) {
    return (
      <div className="worker-chain" data-chain={def.workflow.service}>
        <h4>Steps</h4>
        <p className="muted">
          Dialect {def.workflow.dialect}, on one service: {def.workflow.service}. No model is in this loop.
        </p>
        <ol className="chain">
          {(def.workflow.steps ?? []).map((s) => (
            <li key={s.id} data-step={s.id} data-approval={String(s.approval)}>
              <span className="step-id">{s.id}</span> <code>{s.verb}</code>
              {s.approval && <span className="step-approval"> — approval node: this step becomes a card, never a call</span>}
              {Object.keys(s.needs ?? {}).length > 0 && (
                <ul className="step-needs">
                  {Object.entries(s.needs ?? {}).map(([arg, ref]) => (
                    <li key={arg} data-needs={ref}>
                      <code>{arg}</code> ← <code>{ref}</code>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ol>
      </div>
    )
  }
  return (
    <div className="worker-chain" data-chain="selectors">
      <h4>What selects it</h4>
      {def.workflow_absent !== undefined && def.workflow_absent !== '' && (
        <p className="absent" data-workflow-absent>
          {def.workflow_absent}
        </p>
      )}
      <dl>
        <div>
          <dt>Family</dt>
          <dd>{def.selectors.family ?? <Absent reason="none declared" />}</dd>
        </div>
        <div>
          <dt>Task classes</dt>
          <dd>
            {(def.selectors.task_classes ?? []).length === 0 ? (
              <Absent reason="none declared" />
            ) : (
              <Chips items={def.selectors.task_classes ?? []} />
            )}
          </dd>
        </div>
        <div>
          <dt>Trigger phrases</dt>
          <dd>
            {(def.selectors.triggers ?? []).length === 0 ? (
              <Absent reason="none declared" />
            ) : (
              <Chips items={def.selectors.triggers ?? []} />
            )}
          </dd>
        </div>
        <div>
          <dt>Duty and effort</dt>
          <dd>
            {def.profile.duty ?? <Absent reason="not declared" />}
            {def.profile.effort_floor !== undefined && def.profile.effort_floor !== '' && (
              <> · effort floor {def.profile.effort_floor}</>
            )}
          </dd>
        </div>
        {def.profile.model_pin !== undefined && def.profile.model_pin !== '' && (
          <div>
            <dt>Model pin</dt>
            <dd>
              {def.profile.model_pin}
              {def.profile.model_pin_reason !== undefined && def.profile.model_pin_reason !== '' && (
                <> — {def.profile.model_pin_reason}</>
              )}
            </dd>
          </div>
        )}
      </dl>
    </div>
  )
}

/** One version: provenance, what it was granted, its dated records, and the
 *  runs routed to it. */
function VersionBlock({ version: v }: { version: WorkerVersion }) {
  return (
    <div className="version" data-version={v.version_id} data-active={String(v.active)}>
      <dl className="provenance">
        <div>
          <dt>Version</dt>
          <dd>
            {String(v.version)}
            {v.active && ' — active'}
          </dd>
        </div>
        <div>
          <dt>Authored by</dt>
          <dd>
            {v.author_kind}
            {v.composer !== undefined && v.composer !== '' && <> ({v.composer})</>}
            {v.playbook_version !== undefined && v.playbook_version !== '' && <> · playbook {v.playbook_version}</>}
          </dd>
        </div>
        <div>
          <dt>Origin</dt>
          <dd>
            {v.origin}
            {v.origin_ref !== undefined && v.origin_ref !== '' && <> ({v.origin_ref})</>}
          </dd>
        </div>
        {v.evidence_ref !== undefined && v.evidence_ref !== '' && (
          <div>
            <dt>Evidence</dt>
            <dd>{v.evidence_ref}</dd>
          </div>
        )}
        <div>
          <dt>Approved</dt>
          <dd>
            {v.approved_by === undefined || v.approved_by === '' ? (
              <Absent reason="not approved" />
            ) : (
              <>
                {v.approved_by} · <Stamp ts={v.approved_ts} />
              </>
            )}
          </dd>
        </div>
        <div>
          <dt>Graduated</dt>
          <dd>
            {v.graduated_ts === undefined || v.graduated_ts === '' ? (
              <Absent reason="not graduated — supervised first-N is not complete" />
            ) : (
              <Stamp ts={v.graduated_ts} />
            )}
          </dd>
        </div>
        <div>
          <dt>Definition file</dt>
          <dd>
            <code>{v.file_path}</code> · <code className="sha">{v.file_sha256}</code>
          </dd>
        </div>
      </dl>

      {!v.active && <Grants version={v} />}
      <Records version={v} />
      <Outcomes version={v} />
    </div>
  )
}

/** The dated records: the battery pass that admitted this version, and the
 *  revalidation stamp that returned it to service. A RED stamp renders as one —
 *  it is on the record and the version stays flagged (S14.8 ¶3). */
function Records({ version: v }: { version: WorkerVersion }) {
  return (
    <div className="version-records">
      <p data-validation={v.validation === null ? 'absent' : String(v.validation.green)}>
        Validation:{' '}
        {v.validation === null ? (
          <Absent reason={v.validation_absent ?? 'no validation record'} />
        ) : (
          <>
            {v.validation.green ? 'green' : 'red'} · keyed {v.validation.model === undefined || v.validation.model === '' ? 'on dialect' : v.validation.model}{' '}
            {v.validation.engine_pin} · <Stamp ts={v.validation.created_ts} />
            {v.validation.approver !== undefined && v.validation.approver !== '' && <> · approved by {v.validation.approver}</>}
          </>
        )}
      </p>
      <p data-revalidation={v.revalidation === null ? 'absent' : String(v.revalidation.green)}>
        Revalidation:{' '}
        {v.revalidation === null ? (
          <Absent reason={v.revalidation_absent ?? 'no revalidation stamp'} />
        ) : (
          <>
            {v.revalidation.green ? 'green' : 'red — the version stays flagged'} · {v.revalidation.suites} against{' '}
            {v.revalidation.baseline} · {v.revalidation.actor} · <Stamp ts={v.revalidation.stamped_ts} />
          </>
        )}
      </p>
    </div>
  )
}

/**
 * The per-version quality and cost reading (R14; S08.4 "version→outcome is a
 * JOIN, not a system").
 *
 * Every figure arrives as served. Money in particular is READ off the metering
 * seam per run and is never summed: a per-version total would be arithmetic
 * nobody performed, and it would answer a question this platform does not ask,
 * because each person's account burns separately (D2).
 */
function Outcomes({ version: v }: { version: WorkerVersion }) {
  const runs = v.outcomes.runs ?? []
  return (
    <div className="version-outcomes" data-outcomes={v.version_id}>
      <h5>Runs routed to this version</h5>
      <p className="outcome-tally">
        <span data-runs-routed={String(v.outcomes.runs_routed)}>
          {String(v.outcomes.runs_routed)} routed in this reading
        </span>
        {' · '}
        {(v.outcomes.verdict_tally ?? []).length === 0 ? (
          <Absent reason="no verdict recorded against this version" />
        ) : (
          (v.outcomes.verdict_tally ?? []).map((t) => (
            <span key={t.verdict} className="tally" data-tally={t.verdict} data-rounds={String(t.rounds)}>
              {t.verdict}: {String(t.rounds)}{' '}
            </span>
          ))
        )}
        {/* A bound that truncates has to be visible where the number is. The
            tally is counted from the rounds this reading carries, so once
            either bound cuts, every figure above is a floor. */}
        {v.outcomes.verdict_tally_truncated === true && v.outcomes.verdict_tally.length > 0 && (
          <span className="muted" data-tally-truncated>
            — counted from the rounds in this reading, so these are at least that many
          </span>
        )}
      </p>
      {runs.length === 0 ? (
        // The server's own reason where it gave one — this states what the
        // absence IS and never fills it in.
        <EmptyState
          what={v.outcomes.absent ?? 'no outcomes recorded'}
          why="A row appears here for each run the routing pipeline sent to this version, with the reason it was chosen and what it consumed."
        />
      ) : (
        <>
          <ul className="routed-runs">
            {runs.map((r) => (
              <RoutedRunRow key={`${r.run_id}/${r.routed_ts}`} run={r} />
            ))}
          </ul>
          {v.outcomes.truncated && (
            <p className="muted" data-runs-truncated>
              These are the most recent routed runs, not all of them.
            </p>
          )}
        </>
      )}
    </div>
  )
}

function RoutedRunRow({ run: r }: { run: RoutedRun }) {
  return (
    <li className="routed-run" data-run={r.run_id} data-cause={r.cause}>
      <p className="routed-head">
        <span className="run-id">{r.run_id === '' ? <Absent reason="no run attributed" /> : r.run_id}</span> ·{' '}
        <Owner id={r.owner} /> · <span data-routing-cause>{r.cause}</span> · <Stamp ts={r.routed_ts} />
      </p>
      <p className="routed-why">
        {r.plain_reason === undefined || r.plain_reason === '' ? (
          <Absent reason="no reason was recorded for this routing" />
        ) : (
          r.plain_reason
        )}
      </p>
      <p className="routed-lane">
        {r.model !== undefined && r.model !== '' && <>{r.model} · </>}
        {r.lane !== undefined && r.lane !== '' && <>{r.lane} · </>}
        {r.effort !== undefined && r.effort !== '' && <>effort {r.effort}</>}
        {r.degraded === true && <> · degraded-marked</>}
        {r.generalist === true && <> · ran as the generalist</>}
      </p>
      <p className="routed-verdicts" data-verdicts={String((r.verdicts ?? []).length)}>
        {(r.verdicts ?? []).length === 0 ? (
          <Absent reason={r.verdicts_absent ?? 'no verdict recorded'} />
        ) : (
          <>
            {(r.verdicts ?? []).map((v) => (
              <span key={`${String(v.round)}/${v.ts}`} className="verdict" data-verdict={v.verdict}>
                round {String(v.round)}: {v.verdict}
                {v.revision !== undefined && v.revision !== 0 && <> (revision {String(v.revision)})</>}{' '}
              </span>
            ))}
            {r.verdicts_truncated === true && (
              <span className="muted" data-verdicts-truncated>
                — the most recent rounds, not all of them
              </span>
            )}
          </>
        )}
      </p>
      <p className="routed-cost">
        {r.api_equiv_cost_usd === null ? (
          <Absent reason={r.meter_absent ?? 'no meter reading'} />
        ) : (
          <>
            {/* Same unpriced truth as the task card (review #9): a zero on a
                subscription lane is "no price exists", never an API-equivalent
                figure. */}
            {r.unpriced === true && r.api_equiv_cost_usd === 0 ? (
              <span>subscription lane — no dollar price exists for these calls (UNPRICED)</span>
            ) : (
              <Money usd={r.api_equiv_cost_usd} />
            )}
            {r.tokens !== null && <> · {String(r.tokens)} tokens</>}
            {r.unpriced === true && r.api_equiv_cost_usd !== 0 && (
              <> · API-equivalent estimate — subscription lane, no per-call price</>
            )}
          </>
        )}
      </p>
    </li>
  )
}

/** Chips renders a served list. Every value here is registry content — a tool
 *  name, a topic key, a host — and renders escaped like everything else. The
 *  landed `.chip` class stays as the DOM hook it always was; the look now comes
 *  from the kit's one chip formula. */
function Chips({ items }: { items: string[] }) {
  return (
    <>
      {items.map((s) => (
        <ToneChip key={s} className="chip me-1 normal-case">
          {s}
        </ToneChip>
      ))}
    </>
  )
}

/**
 * byDomain groups the roster the way a person reads it (OQ6's owned rendering),
 * and it is FORWARD-TOLERANT by construction: the grouping key is the served
 * domain string, so a domain this client has never heard of gets its own group
 * labelled with the value the server sent. A worker must never vanish from the
 * map because a client-side list did not anticipate its domain — the same rule
 * the board's kanban vocabulary already follows (§42).
 *
 * Within a domain, automations sort after agentic workers and then by name, so
 * the order is stable across reads rather than following whatever the roster's
 * recency ordering happened to be.
 */
function byDomain(workers: WorkerRow[]): [string, WorkerRow[]][] {
  const groups = new Map<string, WorkerRow[]>()
  for (const w of workers) {
    const key = w.domain.name === '' ? '(no domain recorded)' : w.domain.name
    const list = groups.get(key)
    if (list === undefined) groups.set(key, [w])
    else list.push(w)
  }
  return [...groups.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([domain, list]) => [
      domain,
      [...list].sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind.localeCompare(b.kind))),
    ])
}
