// Package settings is the settings registry of Spec S01.10: every
// operator-editable ⚙ value in the platform, declared exactly once in code
// with its ratified default, clamp, and plain-language help, indexed by Spec
// S18.
//
// Storage is the convergent hybrid: the default lives in code, a
// platform.db row stores only the override, and reset-to-default deletes
// the row. Every write — human or automated — appends a settings_events
// audit row AND emits a settings.changed event on the main event log, in
// one transaction (Spec S01.10; G3 Def.2). Automation may move a value only
// on auto-flagged keys and strictly within the operator-owned (floor,
// ceiling) bounds; only the operator edits bounds (G1 rider 1).
//
// Consumers never hardcode a ⚙ number: they read the registry by dotted key
// (P3/CONVENTIONS.md §2). Until Attach, getters serve the declared
// defaults, which is what bootstrap needs before platform.db is open (Spec
// S01.6 step 1); writes require Attach because an unaudited write must not
// exist.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// EventSettingsChanged is the event type emitted on the main event log for
// every settings write (Spec S01.10; G3 Def.2).
const EventSettingsChanged = "settings.changed"

// settingsEventSchemaVersion versions the settings.changed payload.
const settingsEventSchemaVersion = 1

// Type is a setting's value type. Values have one canonical Go
// representation each; durations canonicalize to whole seconds.
type Type int

const (
	TypeBool     Type = iota + 1 // bool
	TypeInt                      // int64, unit per Decl.Unit
	TypeFloat                    // float64, unit per Decl.Unit
	TypeDuration                 // int64 seconds, exposed as time.Duration
	TypeString                   // string
	TypeEnum                     // string, one of Decl.Enum
	TypeList                     // []string
	TypeMap                      // map[string]string
)

func (t Type) String() string {
	switch t {
	case TypeBool:
		return "bool"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeDuration:
		return "duration"
	case TypeString:
		return "string"
	case TypeEnum:
		return "enum"
	case TypeList:
		return "list"
	case TypeMap:
		return "map"
	}
	return fmt.Sprintf("Type(%d)", int(t))
}

// numeric reports whether values of this type carry (floor, ceiling)
// bounds.
func (t Type) numeric() bool {
	return t == TypeInt || t == TypeFloat || t == TypeDuration
}

// Decl is one declare-once setting (Spec S01.10: key, type, default,
// group/section, title, plain-language help, ⚙ flag, optional bounds,
// restart-required flag — every entry here is ⚙ by construction).
type Decl struct {
	Key     string // dotted, first segment = domain (Spec S18.1)
	Section string // owning spec section, e.g. "S02"
	Title   string
	Help    string // 13.5 plain-language help
	Type    Type
	Unit    string // "seconds", "bytes", "days", ... ; "" = unitless
	Default any    // canonical representation for Type

	// Min/Max is the ratified clamp, in the canonical numeric unit. It
	// seeds the operator-owned bounds (G1 rider 1); nil = unbounded.
	Min, Max     *float64
	MinExclusive bool

	Enum    []string // TypeEnum member set
	Auto    bool     // automation may move the value within bounds (G1 rider 1)
	PerUser bool     // per-user override scope (S18 row qualifier)
	Restart bool     // restart-required (S01.10 flag; S18 column R)
	Dormant string   // non-empty: dormant-at-v0 note (S18.5)

	Ratified string // provenance as carried by the owning ⚙ table
}

// Domain returns the key's domain prefix (first dotted segment).
func (d Decl) Domain() string {
	for i := 0; i < len(d.Key); i++ {
		if d.Key[i] == '.' {
			return d.Key[:i]
		}
	}
	return d.Key
}

// ActorKind classifies a settings writer (G1 rider 1 distinguishes the
// operator from automation).
type ActorKind string

const (
	ActorOperator   ActorKind = "operator"
	ActorAutomation ActorKind = "automation"
)

// Actor identifies who performs a write; it lands verbatim on the audit
// trail.
type Actor struct {
	Kind ActorKind
	ID   string
}

func (a Actor) String() string { return string(a.Kind) + ":" + a.ID }

func (a Actor) validate() error {
	if a.Kind != ActorOperator && a.Kind != ActorAutomation {
		return fmt.Errorf("settings: unknown actor kind %q", a.Kind)
	}
	if a.ID == "" {
		return errors.New("settings: actor id is empty")
	}
	return nil
}

// Errors callers branch on.
var (
	ErrUnknownKey  = errors.New("settings: key not declared")
	ErrNotAttached = errors.New("settings: registry not attached to storage (writes require the audit trail, Spec S01.10)")
	ErrOutOfBounds = errors.New("settings: value outside (floor, ceiling) bounds")
	ErrAutomation  = errors.New("settings: write not permitted for automation (G1 rider 1)")
)

type boundsOverride struct {
	floor, ceiling *float64
}

// Registry is the settings registry. The zero value is unusable; use New.
type Registry struct {
	// writeMu serializes Set/SetBounds/Attach end to end. mu guards the
	// override maps and attachment state and is deliberately NOT held
	// across persistence: the event append inside the write transaction
	// reads ⚙ state.event_payload_cap back through this same registry.
	writeMu sync.Mutex
	mu      sync.RWMutex

	decls map[string]*Decl // immutable after New
	order []string
	rules []rule

	vals     map[string]any            // platform-scope value overrides, canonical
	userVals map[string]map[string]any // key → user → value override
	bounds   map[string]boundsOverride // key → operator-edited bounds

	db  *storage.DB
	log *eventlog.Log
}

// New builds the registry from the declaration index. It panics on an
// inconsistent index — that is a build defect, pinned by this package's
// tests against the S18.5 tallies.
func New() *Registry {
	r := &Registry{
		decls:    make(map[string]*Decl),
		vals:     make(map[string]any),
		userVals: make(map[string]map[string]any),
		bounds:   make(map[string]boundsOverride),
		rules:    rules(),
	}
	for _, d := range index() {
		d := d
		if err := validateDecl(&d); err != nil {
			panic(fmt.Sprintf("settings: bad declaration %q: %v", d.Key, err))
		}
		if _, dup := r.decls[d.Key]; dup {
			panic(fmt.Sprintf("settings: key %q declared twice (declare-once, Spec S01.10)", d.Key))
		}
		r.decls[d.Key] = &d
		r.order = append(r.order, d.Key)
	}
	// Rules may only reference declared keys.
	for _, ru := range r.rules {
		for _, k := range ru.keys {
			if _, ok := r.decls[k]; !ok {
				panic(fmt.Sprintf("settings: rule %q references undeclared key %q", ru.name, k))
			}
		}
	}
	return r
}

func validateDecl(d *Decl) error {
	if d.Key == "" || d.Section == "" || d.Title == "" || d.Help == "" || d.Ratified == "" {
		return errors.New("key, section, title, help and ratified-by are mandatory")
	}
	if d.Domain() == d.Key {
		return errors.New("key has no domain prefix")
	}
	if _, err := checkType(d, d.Default); err != nil {
		return fmt.Errorf("default: %w", err)
	}
	if d.Type == TypeEnum {
		if len(d.Enum) == 0 {
			return errors.New("enum type without members")
		}
	} else if len(d.Enum) != 0 {
		return errors.New("enum members on a non-enum type")
	}
	if !d.Type.numeric() && (d.Min != nil || d.Max != nil) {
		return errors.New("bounds on a non-numeric type")
	}
	if d.Min != nil && d.Max != nil && *d.Min > *d.Max {
		return errors.New("min > max")
	}
	if n, ok := numericOf(d, d.Default); ok {
		if err := checkClamp(d, nil, n); err != nil {
			return fmt.Errorf("default: %w", err)
		}
	}
	if d.Auto && !d.Type.numeric() {
		return errors.New("auto-adjust on a non-numeric type")
	}
	return nil
}

// checkType verifies v against the declaration's canonical representation
// and returns it normalized.
func checkType(d *Decl, v any) (any, error) {
	switch d.Type {
	case TypeBool:
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case TypeInt, TypeDuration:
		if i, ok := v.(int64); ok {
			return i, nil
		}
		if i, ok := v.(int); ok {
			return int64(i), nil
		}
	case TypeFloat:
		if f, ok := v.(float64); ok {
			return f, nil
		}
		if i, ok := v.(int); ok {
			return float64(i), nil
		}
	case TypeString, TypeEnum:
		if s, ok := v.(string); ok {
			if d.Type == TypeEnum && !contains(d.Enum, s) {
				return nil, fmt.Errorf("%q is not one of %v", s, d.Enum)
			}
			return s, nil
		}
	case TypeList:
		if l, ok := v.([]string); ok {
			return append([]string(nil), l...), nil
		}
	case TypeMap:
		if m, ok := v.(map[string]string); ok {
			out := make(map[string]string, len(m))
			for k, vv := range m {
				out[k] = vv
			}
			return out, nil
		}
	default:
		return nil, fmt.Errorf("unknown type %v", d.Type)
	}
	return nil, fmt.Errorf("value %T does not match declared type %v", v, d.Type)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// decodeValue decodes a JSON-encoded write into the canonical
// representation (validation-on-write, Spec S01.10 consumer (a)).
func decodeValue(d *Decl, raw json.RawMessage) (any, error) {
	dec := func(target any) error {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("settings: %s expects a %v value: %w", d.Key, d.Type, err)
		}
		return nil
	}
	switch d.Type {
	case TypeBool:
		var b bool
		if err := dec(&b); err != nil {
			return nil, err
		}
		return b, nil
	case TypeInt, TypeDuration:
		var f float64
		if err := dec(&f); err != nil {
			return nil, err
		}
		if f != math.Trunc(f) {
			return nil, fmt.Errorf("settings: %s expects a whole number, got %v", d.Key, f)
		}
		return int64(f), nil
	case TypeFloat:
		var f float64
		if err := dec(&f); err != nil {
			return nil, err
		}
		return f, nil
	case TypeString, TypeEnum:
		var s string
		if err := dec(&s); err != nil {
			return nil, err
		}
		return checkType(d, s)
	case TypeList:
		var l []string
		if err := dec(&l); err != nil {
			return nil, err
		}
		if l == nil {
			l = []string{}
		}
		return l, nil
	case TypeMap:
		var m map[string]string
		if err := dec(&m); err != nil {
			return nil, err
		}
		if m == nil {
			m = map[string]string{}
		}
		return m, nil
	}
	return nil, fmt.Errorf("settings: unknown type %v", d.Type)
}

// numericOf returns the canonical numeric view of a value for clamp and
// rule checks.
func numericOf(d *Decl, v any) (float64, bool) {
	switch d.Type {
	case TypeInt, TypeDuration:
		return float64(v.(int64)), true
	case TypeFloat:
		return v.(float64), true
	}
	return 0, false
}

// checkClamp validates n against the effective bounds: the operator
// override when present, else the declared clamp (G1 rider 1). A declared
// exclusive floor applies only while no operator floor override is in
// force.
func checkClamp(d *Decl, ov *boundsOverride, n float64) error {
	min, max := d.Min, d.Max
	declaredMin := true
	if ov != nil {
		if ov.floor != nil {
			min, declaredMin = ov.floor, false
		}
		if ov.ceiling != nil {
			max = ov.ceiling
		}
	}
	if min != nil {
		if d.MinExclusive && declaredMin {
			if n <= *min {
				return fmt.Errorf("%w: %v <= floor %v (exclusive)", ErrOutOfBounds, n, *min)
			}
		} else if n < *min {
			return fmt.Errorf("%w: %v < floor %v", ErrOutOfBounds, n, *min)
		}
	}
	if max != nil && n > *max {
		return fmt.Errorf("%w: %v > ceiling %v", ErrOutOfBounds, n, *max)
	}
	return nil
}

// decl returns the declaration or ErrUnknownKey.
func (r *Registry) decl(key string) (*Decl, error) {
	if d, ok := r.decls[key]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownKey, key)
}

// effective returns the canonical effective value at a scope: user override
// (per-user keys only) → platform override → declared default.
func (r *Registry) effective(d *Decl, user string) any {
	if user != "" {
		if uv, ok := r.userVals[d.Key]; ok {
			if v, ok := uv[user]; ok {
				return v
			}
		}
	}
	if v, ok := r.vals[d.Key]; ok {
		return v
	}
	return d.Default
}

func (r *Registry) typed(key, user string, want ...Type) (any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, err := r.decl(key)
	if err != nil {
		return nil, err
	}
	ok := false
	for _, t := range want {
		if d.Type == t {
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("settings: %s is %v, not %v", key, d.Type, want)
	}
	if user != "" && !d.PerUser {
		return nil, fmt.Errorf("settings: %s is not per-user scoped", key)
	}
	return r.effective(d, user), nil
}

// Bool returns the effective value of a bool key.
func (r *Registry) Bool(key string) (bool, error) {
	v, err := r.typed(key, "", TypeBool)
	if err != nil {
		return false, err
	}
	return v.(bool), nil
}

// Int returns the effective value of an int key, in its declared unit.
func (r *Registry) Int(key string) (int64, error) {
	v, err := r.typed(key, "", TypeInt)
	if err != nil {
		return 0, err
	}
	return v.(int64), nil
}

// IntFor is Int resolved for a user (per-user keys).
func (r *Registry) IntFor(key, userID string) (int64, error) {
	v, err := r.typed(key, userID, TypeInt)
	if err != nil {
		return 0, err
	}
	return v.(int64), nil
}

// Float returns the effective value of a float key.
func (r *Registry) Float(key string) (float64, error) {
	v, err := r.typed(key, "", TypeFloat)
	if err != nil {
		return 0, err
	}
	return v.(float64), nil
}

// FloatFor is Float resolved for a user (per-user keys).
func (r *Registry) FloatFor(key, userID string) (float64, error) {
	v, err := r.typed(key, userID, TypeFloat)
	if err != nil {
		return 0, err
	}
	return v.(float64), nil
}

// Duration returns the effective value of a duration key.
func (r *Registry) Duration(key string) (time.Duration, error) {
	v, err := r.typed(key, "", TypeDuration)
	if err != nil {
		return 0, err
	}
	return time.Duration(v.(int64)) * time.Second, nil
}

// String returns the effective value of a string or enum key.
func (r *Registry) String(key string) (string, error) {
	v, err := r.typed(key, "", TypeString, TypeEnum)
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// Strings returns the effective value of a list key (a copy).
func (r *Registry) Strings(key string) ([]string, error) {
	v, err := r.typed(key, "", TypeList)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), v.([]string)...), nil
}

// StringMap returns the effective value of a map key (a copy).
func (r *Registry) StringMap(key string) (map[string]string, error) {
	v, err := r.typed(key, "", TypeMap)
	if err != nil {
		return nil, err
	}
	m := v.(map[string]string)
	out := make(map[string]string, len(m))
	for k, vv := range m {
		out[k] = vv
	}
	return out, nil
}

// Bounds returns the effective (floor, ceiling) of a numeric key: the
// operator override where set, else the declared clamp. nil = unbounded.
func (r *Registry) Bounds(key string) (floor, ceiling *float64, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, err := r.decl(key)
	if err != nil {
		return nil, nil, err
	}
	if !d.Type.numeric() {
		return nil, nil, fmt.Errorf("settings: %s is %v and carries no bounds", key, d.Type)
	}
	floor, ceiling = d.Min, d.Max
	if ov, ok := r.bounds[key]; ok {
		if ov.floor != nil {
			floor = ov.floor
		}
		if ov.ceiling != nil {
			ceiling = ov.ceiling
		}
	}
	return cloneF(floor), cloneF(ceiling), nil
}

func cloneF(f *float64) *float64 {
	if f == nil {
		return nil
	}
	v := *f
	return &v
}

// Decls returns every declaration in index order (copies).
func (r *Registry) Decls() []Decl {
	out := make([]Decl, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, *r.decls[k])
	}
	return out
}

// Decl returns one declaration by key.
func (r *Registry) Decl(key string) (Decl, bool) {
	d, ok := r.decls[key]
	if !ok {
		return Decl{}, false
	}
	return *d, true
}

// Domains returns the sorted set of domain prefixes.
func (r *Registry) Domains() []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range r.order {
		dom := r.decls[k].Domain()
		if !seen[dom] {
			seen[dom] = true
			out = append(out, dom)
		}
	}
	sort.Strings(out)
	return out
}

// view is the hypothetical post-write effective state rules check against.
type view struct {
	r       *Registry
	overlay map[string]any
}

func (v *view) value(key string) any {
	if ov, ok := v.overlay[key]; ok {
		return ov
	}
	return v.r.effective(v.r.decls[key], "")
}

func (v *view) float(key string) float64 {
	n, _ := numericOf(v.r.decls[key], v.value(key))
	return n
}

func (v *view) str(key string) string { return v.value(key).(string) }

func (v *view) strings(key string) []string { return v.value(key).([]string) }

func (v *view) stringMap(key string) map[string]string {
	return v.value(key).(map[string]string)
}

// checkRules runs every relational rule against the hypothetical state
// (Spec S18.2 relational clamps; sweep rules S18.4).
func (r *Registry) checkRules(overlay map[string]any) error {
	v := &view{r: r, overlay: overlay}
	for _, ru := range r.rules {
		if err := ru.check(v); err != nil {
			return fmt.Errorf("settings: rule %q: %w", ru.name, err)
		}
	}
	return nil
}

type rule struct {
	name  string
	keys  []string
	check func(v *view) error
}

func canonicalJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		// Canonical values are always marshalable; a failure is a build
		// defect surfaced by tests.
		panic(fmt.Sprintf("settings: marshal canonical value: %v", err))
	}
	return b
}
