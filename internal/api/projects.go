package api

import "context"

// projects.go — the S13.7 projects family (P3-RW-2).
//
// RED WINDOW (CONVENTIONS §3 Amendment-A): this file currently carries only the
// INERT type surface the packet's failing acceptance tests need in order to
// COMPILE. No route is registered and no behavior exists; the packet's
// implementation commit closes the window.

// OnboardRefs names the durable objects one onboarding rides: the task the
// platform performs it as, and the ask its owner-approval card lands on (Spec
// S13.7). Both names are internal/stage's — the transport asks for them rather
// than composing them, so one id scheme exists.
type OnboardRefs struct {
	TaskID string `json:"task_id"`
	AskRef string `json:"ask_ref"`
}

// OnboardSurface is the api-facing door to the S13.7 onboarding task (the
// IntakeSurface/CancelSurface/ResumeSurface precedent): *stage.Surface
// implements it at the composition root.
type OnboardSurface interface {
	// StartOnboarding runs the platform's onboarding task for a NEW project:
	// register → initialize the store → scan → draft, then the run whose durable
	// ask carries the draft for the owner's D10 approval.
	StartOnboarding(ctx context.Context, owner, projectID, name, remoteURL string) (OnboardRefs, error)
	// OnboardRefs is the pure naming half: what an already-in-flight onboarding
	// of this project is called. It performs nothing.
	OnboardRefs(projectID string) OnboardRefs
}
