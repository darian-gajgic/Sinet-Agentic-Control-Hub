package broker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
)

// client.go is the broker client: control-plane / run-side dials the per-user
// UDS and submits typed operations. The control plane holds a resolved engine
// credential only transiently, in memory, to inject it into the engine child
// at spawn (S01.6) — never to the event log or platform.db (D2/S11.5). Owner
// secrets are never resolved here at all; they are used via Sign (result-only).

// Client is a broker connection. Safe for sequential use; one op at a time.
type Client struct {
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

// Dial connects to the broker's per-user UDS.
func Dial(socket string) (*Client, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("broker: dial %s: %w", socket, err)
	}
	return &Client{conn: conn, r: bufio.NewReader(conn)}, nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) roundTrip(req Request) (Response, error) {
	resp, err := c.send(req)
	if err != nil {
		return Response{}, err
	}
	if !resp.OK {
		return resp, fmt.Errorf("broker: %s", resp.Error)
	}
	return resp, nil
}

// send is roundTrip WITHOUT the !OK→error collapse: it returns the raw
// response so a caller (Push) can distinguish a stale-lease rejection from a
// broker refusal. A returned error is a wire/transport failure only.
func (c *Client) send(req Request) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := writeJSON(c.conn, req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := readJSONReader(c.r, &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Store writes a credential (admin/setup op; the server must have AllowStore).
func (c *Client) Store(profile, kind, secret string) error {
	_, err := c.roundTrip(Request{Op: OpStore, Profile: profile, Kind: kind, Secret: secret})
	return err
}

// Resolve returns an engine credential for spawn-time injection (S11.5). The
// caller injects it into the engine child's env and discards it — it is never
// persisted or logged.
func (c *Client) Resolve(profile string) (secret, kind string, err error) {
	resp, err := c.roundTrip(Request{Op: OpResolve, Profile: profile})
	if err != nil {
		return "", "", err
	}
	return resp.Secret, resp.Kind, nil
}

// Sign executes an owner signing-key WITH the secret and returns only the
// HMAC — the key never leaves the broker (S11.5 ssh-agent posture).
func (c *Client) Sign(profile string, data []byte) ([]byte, error) {
	resp, err := c.roundTrip(Request{Op: OpSign, Profile: profile, Data: base64.StdEncoding.EncodeToString(data)})
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(resp.Sig)
	if err != nil {
		return nil, fmt.Errorf("broker: decode signature: %w", err)
	}
	return sig, nil
}

// PushResult is the outcome of a broker CAS push (Spec S13.6).
type PushResult struct {
	// Rejected is true when git rejected the push because the
	// --force-with-lease lease was stale (the ref moved since approval) — a
	// NORMAL collision the caller routes back to a merge card, NEVER a blind
	// retry against a new sha (S13.6 step 4).
	Rejected bool
}

// Push runs a CAS push through the broker (Spec S13.6). Its distinctions
// matter to the caller: a broker REFUSAL (a protected ref without
// accept-authorization, or a bare / value-less lease) returns an error — the
// P-T12-1 guardrail held; a stale-lease REJECTION returns
// PushResult{Rejected:true}, nil so the accept path routes to a merge card;
// success returns a zero PushResult. The caller sets Op-independent fields
// (RepoDir/Remote/Refs/Protected/Authorized/Atomic/Profile) on req.
func (c *Client) Push(req Request) (PushResult, error) {
	req.Op = OpPush
	resp, err := c.send(req)
	if err != nil {
		return PushResult{}, err
	}
	if resp.Rejected {
		return PushResult{Rejected: true}, nil
	}
	if !resp.OK {
		return PushResult{}, fmt.Errorf("broker: %s", resp.Error)
	}
	return PushResult{}, nil
}

// SignData produces an SSHSIG over data with the broker-held git-ssh-key (Spec
// S13.6 step 5, gpg.format=ssh). Returns the armored signature; the key never
// leaves the broker. namespace is "git" for commit signing.
func (c *Client) SignData(profile, namespace string, data []byte) ([]byte, error) {
	resp, err := c.roundTrip(Request{
		Op: OpSignData, Profile: profile, Namespace: namespace,
		Data: base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(resp.Sig)
	if err != nil {
		return nil, fmt.Errorf("broker: decode signature: %w", err)
	}
	return sig, nil
}

// EnvInjector returns an adapters.StartRequest.CredInject closure that
// resolves an engine credential from the broker at spawn and appends it to the
// engine environment as varName. Fresh per spawn; the secret is never stored
// on the StartRequest, in a park record, in logs, or in the DB (S11.5). For a
// CONFINED engine, pass a sentinel var name / value contract instead — the
// real token then rides the injection proxy (D2/S11.5); this injector is the
// outside-the-sandbox spawn path (S01.6/S01.2).
func EnvInjector(socket, profile, varName string) func(base []string) ([]string, error) {
	return func(base []string) ([]string, error) {
		c, err := Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		secret, _, err := c.Resolve(profile)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(base)+1)
		out = append(out, base...)
		out = append(out, varName+"="+secret)
		return out, nil
	}
}

// ── shared JSON framing (newline-delimited) ─────────────────────────────────

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

func readJSONReader(r *bufio.Reader, v any) error {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}
