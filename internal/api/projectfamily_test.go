package api_test

// projectfamily_test.go — P3-RW-11 at the transport (R2/R10; Spec S13.7,
// S15.2). The create door carries the owner-declared task family, and a door
// that cannot carry one REFUSES rather than dropping it.
//
// It composes its own server rather than reusing projEnv, because the landed
// fixture's seam deliberately implements only the landed interface — which is
// exactly the "door without the capability" this file also has to drive, and a
// fixture cannot be both.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// familyDoor records what the transport handed the seam. The plain door embeds
// it WITHOUT the family method, so the two postures differ in exactly one thing.
type plainDoor struct {
	proj    *project.Store
	gotName string
}

func (d *plainDoor) OnboardRefs(projectID string) api.OnboardRefs {
	return api.OnboardRefs{TaskID: "onboard-" + projectID, AskRef: "onboard:" + projectID}
}

func (d *plainDoor) StartOnboarding(ctx context.Context, owner, projectID, name, remoteURL string) (api.OnboardRefs, error) {
	return d.start(ctx, owner, projectID, name, remoteURL, "")
}

func (d *plainDoor) start(ctx context.Context, owner, projectID, name, remoteURL, family string) (api.OnboardRefs, error) {
	d.gotName = name
	if _, err := d.proj.OnboardStart(ctx, project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: name, RemoteURL: remoteURL, Family: family,
	}); err != nil {
		if strings.Contains(err.Error(), "invalid input") {
			return api.OnboardRefs{}, &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
		}
		return api.OnboardRefs{}, err
	}
	return d.OnboardRefs(projectID), nil
}

// familyDoor is the same door with the additive half wired.
type familyDoor struct{ *plainDoor }

func (d familyDoor) StartOnboardingWithFamily(ctx context.Context, owner, projectID, name, remoteURL, family string) (api.OnboardRefs, error) {
	return d.start(ctx, owner, projectID, name, remoteURL, family)
}

var (
	_ api.OnboardSurface       = (*plainDoor)(nil)
	_ api.OnboardFamilySurface = familyDoor{}
)

func familyServer(t *testing.T, who string, door api.OnboardSurface, b *backend) *api.Server {
	t.Helper()
	return api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:       b.db, Meter: fakeMeter{}, Onboard: door,
	})
}

func familyEnv(t *testing.T) (*backend, *project.Store) {
	t.Helper()
	b := newBackend(t)
	ctx := context.Background()
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, dlvPIN); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := b.store.CreateUser(ctx, "op", auth.User{ID: "alice", DisplayName: "alice", Role: auth.RoleMember}, dlvPIN); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	proj, err := project.New(project.Config{DB: b.db, Log: b.log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	return b, proj
}

func post(t *testing.T, s *api.Server, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/projects", strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// TestCreateDoorCarriesTheTaskFamily: the declared family reaches the registry
// through the door and lands on the entry's capture.
func TestCreateDoorCarriesTheTaskFamily(t *testing.T) {
	b, proj := familyEnv(t)
	srv := familyServer(t, "alice", familyDoor{&plainDoor{proj: proj}}, b)

	code, out := post(t, srv, `{"project_id":"shop","name":"Shop backend","family":"software"}`)
	if code != http.StatusOK {
		t.Fatalf("POST with a family: %d %s", code, out)
	}
	e, err := proj.Get(context.Background(), "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.Capture.Family != project.FamilySoftware {
		t.Errorf("capture family = %q, want software", e.Capture.Family)
	}
}

// TestCreateDoorRefusesAnUnknownFamily: the transport declares the field and
// none of its values — the registry's own refusal is what the caller reads.
func TestCreateDoorRefusesAnUnknownFamily(t *testing.T) {
	b, proj := familyEnv(t)
	srv := familyServer(t, "alice", familyDoor{&plainDoor{proj: proj}}, b)

	code, out := post(t, srv, `{"project_id":"shop","name":"Shop backend","family":"webshop"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("POST with an unknown family: %d %s (want 400)", code, out)
	}
	if !strings.Contains(out, "software") {
		t.Errorf("the refusal does not name the vocabulary it accepts: %s", out)
	}
	if _, err := proj.Get(context.Background(), "shop"); err == nil {
		t.Error("a refused family still registered the project")
	}
}

// TestCreateDoorRefusesAFamilyItCannotDeliver: a door composed without the
// additive half says so instead of onboarding the project family-less — the
// caller asked for a question set and would otherwise never learn they did not
// get one.
func TestCreateDoorRefusesAFamilyItCannotDeliver(t *testing.T) {
	b, proj := familyEnv(t)
	srv := familyServer(t, "alice", &plainDoor{proj: proj}, b)

	code, out := post(t, srv, `{"project_id":"shop","name":"Shop backend","family":"software"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("POST with a family to a plain door: %d %s (want 400)", code, out)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode refusal %q: %v", out, err)
	}
	if body.Error != "family_unsupported" {
		t.Errorf("refusal code = %q, want family_unsupported", body.Error)
	}
	if _, err := proj.Get(context.Background(), "shop"); err == nil {
		t.Error("the project was onboarded WITHOUT the family the caller asked for")
	}
}

// TestCreateDoorWithoutAFamilyIsUnchanged (R10): a body that names no family
// takes the landed path and registers exactly as before, on either door.
func TestCreateDoorWithoutAFamilyIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		door func(*project.Store) api.OnboardSurface
	}{
		{"plain door", func(p *project.Store) api.OnboardSurface { return &plainDoor{proj: p} }},
		{"family-capable door", func(p *project.Store) api.OnboardSurface { return familyDoor{&plainDoor{proj: p}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, proj := familyEnv(t)
			srv := familyServer(t, "alice", tc.door(proj), b)
			code, out := post(t, srv, `{"project_id":"shop","name":"Shop backend"}`)
			if code != http.StatusOK {
				t.Fatalf("POST without a family: %d %s", code, out)
			}
			e, err := proj.Get(context.Background(), "shop")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if e.Capture.Family != "" {
				t.Errorf("capture family = %q, want \"\" (honest absence)", e.Capture.Family)
			}
		})
	}
}
