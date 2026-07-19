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
)

// Request is one typed broker operation.
type Request struct {
	Op      string `json:"op"`
	Profile string `json:"profile"`
	// Kind + Secret are the store op's payload only.
	Kind   string `json:"kind,omitempty"`
	Secret string `json:"secret,omitempty"`
	// Data is the sign op's payload (base64).
	Data string `json:"data,omitempty"`
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
	// Sig is the sign reply (base64 HMAC); the signing key is never returned.
	Sig string `json:"sig,omitempty"`
}
