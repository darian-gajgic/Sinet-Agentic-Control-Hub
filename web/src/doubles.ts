import { vi } from 'vitest'

import type { EventSourceLike } from './events'

import catalogRaw from './fixtures/api/history-catalog.json?raw'
import queryAnswerRaw from './fixtures/api/history-query-answer.json?raw'
import viewAnswerRaw from './fixtures/api/history-view-answer.json?raw'
import viewsRaw from './fixtures/api/history-views.json?raw'
import metersRaw from './fixtures/api/meters.json?raw'
import runsRaw from './fixtures/api/runs.json?raw'
import tasksRaw from './fixtures/api/tasks.json?raw'

/**
 * The offline harness every view test drives (P3/CONVENTIONS.md §42).
 *
 * Nothing here opens a connection to anything: no test dials 8481/8482, the
 * live front chain, any unit, or the network, and VITE_SINET_ADDR stays unset.
 *
 * THE BODIES ARE THE GOLDEN FIXTURES. `fixtures.tasks()` is the byte-for-byte
 * JSON an internal/api test asserts `GET /api/tasks` still serves. So a Go
 * handler that renames a field does not quietly become `undefined` in a view —
 * it fails the GO suite, in the same commit, naming the read. One shape, two
 * consumers.
 */

/** A scripted EventSource, in the landed shape (events.test.ts). */
export class FakeSource implements EventSourceLike {
  static made: FakeSource[] = []
  readonly url: string
  readyState = 0
  private listeners = new Map<string, ((ev: Event) => void)[]>()

  constructor(url: string) {
    this.url = url
    FakeSource.made.push(this)
  }

  static reset(): void {
    FakeSource.made = []
  }

  static last(): FakeSource {
    return FakeSource.made[FakeSource.made.length - 1]
  }

  addEventListener(type: string, listener: (ev: Event) => void): void {
    const list = this.listeners.get(type) ?? []
    list.push(listener)
    this.listeners.set(type, list)
  }

  close(): void {
    this.readyState = 2
  }

  open(): void {
    this.readyState = 1
    this.fire(new Event('open'))
  }

  /** send delivers one delta frame under its stored event type — the transport
   *  names every frame with its type, which is why subscriptions declare the
   *  types they consume. */
  send(type: string, data: unknown): void {
    this.fire(new MessageEvent(type, { data: JSON.stringify(data) }))
  }

  private fire(ev: Event): void {
    for (const l of this.listeners.get(ev.type) ?? []) l(ev)
  }
}

/** One frame, with the fields the client routes on. */
export function frame(seq: number, type: string, topics: string[] = ['board']) {
  return { seq, user_id: 'alice', type, schema_version: 1, topics, payload: {}, ts: '2026-07-20T09:05:00Z' }
}

export type Scripted = { body?: unknown; status?: number }

export type FetchLog = {
  calls: { method: string; path: string; body: unknown }[]
  /** Replace one route's answer mid-test, to prove a re-read really re-reads. */
  set(key: string, route: Scripted): void
}

/**
 * scriptedFetch installs a control plane made of a routing table. An unscripted
 * request FAILS the test loudly rather than resolving to undefined — a view
 * that quietly calls something nobody scripted is exactly the drift these
 * tests exist to catch.
 */
export function scriptedFetch(table: Record<string, Scripted>): FetchLog {
  const routes = { ...table }
  const calls: { method: string; path: string; body: unknown }[] = []
  const impl = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input)
    const method = init?.method ?? 'GET'
    calls.push({ method, path, body: init?.body ? JSON.parse(String(init.body)) : undefined })
    const route = routes[`${method} ${path}`]
    if (!route) throw new Error(`unscripted request: ${method} ${path}`)
    const status = route.status ?? 200
    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: '',
      json: async () => route.body ?? {},
    } as Response
  })
  vi.stubGlobal('fetch', impl)
  return {
    calls,
    set(key, route) {
      routes[key] = route
    },
  }
}

/** signedIn is the session every view test runs under: an operator, so the
 *  household altitude (every owner's rows) is what the views receive. */
export const signedIn: Scripted = {
  body: { authenticated: true, user: { user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true } },
}

function parse<T>(raw: string): T {
  return JSON.parse(raw) as T
}

/** Fresh parses each call, so one test mutating a body cannot reach another. */
export const fixtures = {
  tasks: () => parse<Record<string, unknown>>(tasksRaw),
  runs: () => parse<Record<string, unknown>>(runsRaw),
  meters: () => parse<Record<string, unknown>>(metersRaw),
  historyViews: () => parse<Record<string, unknown>>(viewsRaw),
  historyCatalog: () => parse<Record<string, unknown>>(catalogRaw),
  historyViewAnswer: () => parse<Record<string, unknown>>(viewAnswerRaw),
  historyQueryAnswer: () => parse<Record<string, unknown>>(queryAnswerRaw),
}

/** The routes the B6-5 oversight surfaces read, answered from the fixtures. */
export function oversightRoutes(): Record<string, Scripted> {
  return {
    'GET /api/auth/session': signedIn,
    'GET /api/tasks': { body: fixtures.tasks() },
    'GET /api/runs': { body: fixtures.runs() },
    'GET /api/meters': { body: fixtures.meters() },
    'GET /api/events/views': { body: fixtures.historyViews() },
    'GET /api/events/catalog': { body: fixtures.historyCatalog() },
  }
}
