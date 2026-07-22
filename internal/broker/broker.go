package broker

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// broker.go is the broker daemon: the per-user UDS listener, the SO_PEERCRED
// attestation, and the policy-decision/credential-delivery split (S11.5). The
// decision path (may this attested caller run this op on this profile?) holds
// ZERO secrets; only delivery touches plaintext.

// Server serves one person's credential store over a per-user UDS. The broker
// process runs one Server per person (one uid, one socket, one Store).
type Server struct {
	store    *Store
	ownerUID uint32
	log      *slog.Logger

	// AllowStore gates the admin store op over the socket. Default false: in
	// production, credentials are migrated in by the operator at a gate, not
	// over the live socket (the packet's credential-hygiene rule). Tests and
	// the dev bring-up enable it explicitly.
	AllowStore bool
}

// NewServer builds a Server. ownerUID is the only uid whose connections are
// honored (SO_PEERCRED attestation; a per-user UDS + this check are S01.9
// layer-equivalent for the broker).
func NewServer(store *Store, ownerUID uint32, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{store: store, ownerUID: ownerUID, log: log}
}

// Listen opens the per-user UDS at path (0600 socket in a 0700 dir — host-
// only, S11.5). A stale socket file is removed first (the broker is the sole
// owner of its socket path).
func Listen(path string) (*net.UnixListener, error) {
	if err := os.MkdirAll(dirOf(path), 0o700); err != nil {
		return nil, fmt.Errorf("broker: socket dir: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("broker: clear stale socket: %w", err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("broker: listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("broker: socket perms: %w", err)
	}
	return ln, nil
}

// Serve accepts connections until ctx is canceled or the listener closes.
func (s *Server) Serve(ctx context.Context, ln *net.UnixListener) error {
	go func() {
		<-ctx.Done()
		ln.Close() // unblocks Accept
	}()
	for {
		conn, err := ln.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("broker: accept: %w", err)
		}
		go s.handle(conn)
	}
}

// handle attests the peer by SO_PEERCRED, then serves typed operations on the
// connection until it closes.
func (s *Server) handle(conn *net.UnixConn) {
	defer conn.Close()
	uid, err := peerUID(conn)
	if err != nil {
		s.log.Warn("broker: peer credential read failed", "err", err)
		return
	}
	if uid != s.ownerUID {
		// Authenticate the asker (S11.5): a foreign uid is refused before any
		// operation is considered — the socket is per-user, this is the
		// belt-and-suspenders check.
		s.log.Warn("broker: rejected non-owner peer", "peer_uid", uid, "owner_uid", s.ownerUID)
		_ = writeJSON(conn, Response{OK: false, Error: "unauthorized"})
		return
	}
	r := bufio.NewReader(conn) // one reader across the connection's frames
	for {
		var req Request
		if err := readJSONReader(r, &req); err != nil {
			return // EOF or malformed frame ends the connection
		}
		resp := s.dispatch(req)
		if err := writeJSON(conn, resp); err != nil {
			return
		}
	}
}

// dispatch is the policy-decision + delivery split (S11.5): the switch and its
// kind/destination checks are the decision path (no secret in scope); only the
// resolve/sign arms touch plaintext, and each enforces the credential's
// destination constraint (a resolve only serves an engine-cred; a sign only a
// signing-key) before delivering.
func (s *Server) dispatch(req Request) Response {
	switch req.Op {
	case OpStore:
		if !s.AllowStore {
			return Response{OK: false, Error: "store operation not permitted over the socket"}
		}
		if err := s.store.Put(req.Profile, req.Kind, req.Secret); err != nil {
			return errResp(err)
		}
		return Response{OK: true}

	case OpResolve:
		kind, secret, err := s.store.resolve(req.Profile)
		if err != nil {
			return errResp(err)
		}
		if kind != KindEngineCred {
			// Destination constraint (S11.5): the injection path serves ONLY
			// engine credentials; owner signing-keys never leave via resolve.
			return errResp(fmt.Errorf("%w: resolve of a %s", ErrWrongKind, kind))
		}
		return Response{OK: true, Kind: kind, Secret: secret}

	case OpSign:
		kind, key, err := s.store.resolve(req.Profile)
		if err != nil {
			return errResp(err)
		}
		if kind != KindSigningKey {
			return errResp(fmt.Errorf("%w: sign with a %s", ErrWrongKind, kind))
		}
		data, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			return errResp(fmt.Errorf("broker: decode sign payload: %w", err))
		}
		keyBytes := []byte(key)
		mac := hmac.New(sha256.New, keyBytes)
		mac.Write(data)
		sig := mac.Sum(nil)
		zero(keyBytes) // shorten the plaintext window; the key is NEVER returned
		return Response{OK: true, Sig: base64.StdEncoding.EncodeToString(sig)}

	case OpPush:
		// The S13.6 CAS push — THE guardrail (P-T12-1). The decision (bare-force
		// refusal, protected-ref-requires-authorization) is in gitPush, which
		// checks BEFORE resolving any credential.
		return s.gitPush(req)

	case OpSignData:
		// The S13.6 SSHSIG signing (gpg.format=ssh); the key never leaves.
		return s.signData(req)

	case OpHasKey:
		// Secret-free presence check for the signing-posture derivation (F3):
		// the decision path holds no secret — kindOf never decrypts.
		kind, exists, err := s.store.kindOf(req.Profile)
		if err != nil {
			return errResp(err)
		}
		return Response{OK: true, Has: exists, Kind: kind}

	default:
		return Response{OK: false, Error: "unknown operation"}
	}
}

// errResp collapses an error to a coarse wire message; the caller's log holds
// the precise cause (the S01.9 collapsed-failure precedent).
func errResp(err error) Response {
	switch {
	case errors.Is(err, ErrNoProfile):
		return Response{OK: false, Error: "no such profile"}
	case errors.Is(err, ErrWrongKind):
		return Response{OK: false, Error: "operation not permitted for this credential"}
	case errors.Is(err, ErrBadProfile):
		return Response{OK: false, Error: "invalid profile"}
	default:
		return Response{OK: false, Error: "broker error"}
	}
}

// peerUID reads the connecting process's uid via SO_PEERCRED — the broker
// authenticates the asker, it never trusts a claimed identity (S11.5).
func peerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var ucred *unix.Ucred
	var opErr error
	if err := raw.Control(func(fd uintptr) {
		ucred, opErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if opErr != nil {
		return 0, opErr
	}
	return ucred.Uid, nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
