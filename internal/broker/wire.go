package broker

// wire.go is the broker's typed operation protocol (Spec S11.5: "the sandbox
// submits a typed operation ... returns only the result"). It is
// newline-delimited JSON over the per-user UDS. The protocol carries NO secret
// on the request side except the admin store op (an operator setup path, not
// agent-reachable); resolve returns an engine credential for spawn injection,
// sign returns only a result and never the key.

// Operation names.
const (
	// OpStore writes a credential under a profile (admin/setup — the operator
	// migrating a key at a future gate; never an agent path).
	OpStore = "store"
	// OpResolve returns an engine-cred for spawn-time injection (S11.5 engine
	// credential injection; S01.6 "engines receive credentials at start").
	OpResolve = "resolve"
	// OpSign executes an owner signing-key WITH the secret and returns only
	// the HMAC — the ssh-agent posture, the key never leaves (S11.5).
	OpSign = "sign"
	// OpPush runs `git push` with the broker-held git-ssh-key (S13.6 step 4;
	// S11.5 "execute with the owner's credential OUTSIDE the sandbox and
	// return only the result"). The broker is THE GUARDRAIL (P-T12-1): every
	// push is CAS (--force-with-lease=<ref>:<expect>; the bare form is
	// forbidden), and a push to a protected ref is refused unless it carries
	// the accept-authorization the control-plane accept/snapshot path is the
	// sole setter of. The git-ssh-key NEVER leaves the broker.
	OpPush = "push"
	// OpSignData produces an SSHSIG over data with the broker-held git-ssh-key
	// (S13.6 step 5, gpg.format=ssh): the key NEVER leaves — only the armored
	// signature is returned. The platform embeds it as the commit's gpgsig
	// header (the accept-commit signing path).
	OpSignData = "sign-data"
)

// RefUpdate is one ref this push moves under CAS. ExpectSHA is the
// --force-with-lease lease value and MUST be present — the bare
// --force-with-lease is forbidden (trivially defeated by background ref
// updates, P-T12-1). SrcSHA is the local commit the ref is set to.
type RefUpdate struct {
	Ref       string `json:"ref"`
	ExpectSHA string `json:"expect_sha"`
	SrcSHA    string `json:"src_sha"`
}

// Request is one typed broker operation.
type Request struct {
	Op      string `json:"op"`
	Profile string `json:"profile"`
	// Kind + Secret are the store op's payload only.
	Kind   string `json:"kind,omitempty"`
	Secret string `json:"secret,omitempty"`
	// Data is the sign/sign-data op's payload (base64).
	Data string `json:"data,omitempty"`
	// Namespace is the sign-data SSHSIG namespace ("git" for commit signing).
	Namespace string `json:"namespace,omitempty"`
	// Push-op fields (S13.6). RepoDir is the local store to push FROM; Remote
	// the remote URL (file:// dev fixtures, ssh:// live — stored data). Refs
	// are the CAS ref updates. Protected is the S13.7 protected-ref policy
	// (registry data, read control-plane-side and handed here — the broker is
	// DB-free). Authorized is the accept-authorization: true ONLY when the
	// control-plane accept/snapshot push path composed this request; a push to
	// a protected ref without it is refused (P-T12-1). Atomic maps to
	// --atomic for multi-ref updates.
	RepoDir    string      `json:"repo_dir,omitempty"`
	Remote     string      `json:"remote,omitempty"`
	Refs       []RefUpdate `json:"refs,omitempty"`
	Protected  []string    `json:"protected,omitempty"`
	Authorized bool        `json:"authorized,omitempty"`
	Atomic     bool        `json:"atomic,omitempty"`
}

// Response is the broker's reply. Error is set (OK false) on any failure; the
// error text is deliberately coarse (the event log, not the wire, carries
// precise reasons — the S01.9 collapsed-failure precedent).
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Secret + Kind are the resolve reply (engine-cred delivery only).
	Secret string `json:"secret,omitempty"`
	Kind   string `json:"kind,omitempty"`
	// Sig is the sign reply (base64 HMAC) / the sign-data reply (the armored
	// SSHSIG); the signing key is never returned.
	Sig string `json:"sig,omitempty"`
	// Rejected marks a push that git rejected because the --force-with-lease
	// lease was stale (the ref moved since approval) — a NORMAL collision the
	// control plane routes back to a merge card, NEVER a blind retry against a
	// new sha (S13.6 step 4). Distinct from a broker refusal (OK false with no
	// Rejected) and from a generic failure.
	Rejected bool `json:"rejected,omitempty"`
}
