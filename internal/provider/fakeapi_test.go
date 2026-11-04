package provider

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"time"
)

// fakeAPI imitates the parts of the Crontinel configuration API the provider uses: upsert by stable key
// (201 on create, 200 on update), a version per record, if_version refusal with 409, 404 for unknown ids,
// and the response shapes seen against the real service. It exists so every change to the provider is
// checked by real Terraform runs in CI; the real API was exercised by hand and the shapes copied from it.
type fakeAPI struct {
	*httptest.Server
	mu      sync.Mutex
	next    int
	records map[string]map[string]record // "app/collection" -> id -> record
}

type record map[string]any

const fakeKey = "ct_mcp_fake"

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{records: map[string]map[string]record{}, next: 100}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

type collectionSpec struct {
	keys    []string // stable key fields
	intID   bool     // integer ids (monitors) instead of uuids (settings)
	shape   func(in record, created bool) (record, *apiProblem)
	ignored []string // fields not part of the version
}

type apiProblem struct {
	status int
	code   string
	msg    string
}

var fakeCollections = map[string]collectionSpec{
	"cron-schedules":      {keys: []string{"command"}, intID: true, ignored: []string{"ping_url", "activated_at"}, shape: shapeCron},
	"heartbeats":          {keys: []string{"name"}, intID: true, shape: shapeHeartbeat},
	"outcome-rules":       {keys: []string{"command", "job_name", "environment", "expression", "key", "kind"}, shape: shapeRule},
	"maintenance-windows": {keys: []string{"name"}, shape: shapeWindow},
}

func versionOf(r record, ignored []string) string {
	clean := map[string]any{}
	for k, v := range r {
		skip := k == "id" || k == "version"
		for _, i := range ignored {
			skip = skip || k == i
		}
		if !skip {
			clean[k] = v
		}
	}
	b, _ := json.Marshal(clean) // encoding/json sorts map keys
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])[:16]
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+fakeKey {
		writeFake(w, 401, map[string]any{"error": "unauthenticated", "message": "Use a generated API key."})
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/apps/"), "/"), "/")
	if len(parts) < 2 {
		writeFake(w, 404, map[string]any{"error": "not_found", "message": "Not found."})
		return
	}
	app, coll := parts[0], parts[1]
	if coll == "escalation" {
		f.escalation(w, r, app)
		return
	}
	spec, ok := fakeCollections[coll]
	if !ok {
		writeFake(w, 404, map[string]any{"error": "not_found", "message": "Not found."})
		return
	}
	store := f.store(app, coll)
	switch {
	case len(parts) == 2 && r.Method == "PUT":
		f.upsert(w, r, store, spec)
	case len(parts) == 3 && r.Method == "GET":
		if rec, ok := store[parts[2]]; ok {
			shown := record{}
			for k, v := range rec {
				if k != "ping_url" { // a credential: the real API shows it once, at creation
					shown[k] = v
				}
			}
			writeFake(w, 200, map[string]any{"data": shown})
			return
		}
		writeFake(w, 404, map[string]any{"error": "not_found", "message": "Resource not found."})
	case len(parts) == 3 && r.Method == "DELETE":
		if _, ok := store[parts[2]]; !ok {
			writeFake(w, 404, map[string]any{"error": "not_found", "message": "Resource not found."})
			return
		}
		delete(store, parts[2])
		w.WriteHeader(204)
	default:
		writeFake(w, 405, map[string]any{"error": "method_not_allowed", "message": "Method not allowed."})
	}
}

func (f *fakeAPI) store(app, coll string) map[string]record {
	k := app + "/" + coll
	if f.records[k] == nil {
		f.records[k] = map[string]record{}
	}
	return f.records[k]
}

func (f *fakeAPI) upsert(w http.ResponseWriter, r *http.Request, store map[string]record, spec collectionSpec) {
	var in record
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeFake(w, 422, map[string]any{"error": "validation_failed", "message": "Invalid body."})
		return
	}
	ifVersion, _ := in["if_version"].(string)
	delete(in, "if_version")

	var existingID string
	for id, rec := range store {
		same := true
		for _, k := range spec.keys {
			if fmt.Sprint(rec[k]) != fmt.Sprint(in[k]) {
				same = false
			}
		}
		if same {
			existingID = id
		}
	}
	if ifVersion != "" && (existingID == "" || store[existingID]["version"] != ifVersion) {
		writeFake(w, 409, map[string]any{"error": "conflict", "message": "This record changed since you read it. Read it again and retry with the new version."})
		return
	}
	rec, problem := spec.shape(in, existingID == "")
	if problem != nil {
		writeFake(w, problem.status, map[string]any{"error": problem.code, "message": problem.msg, "errors": map[string][]string{"input": {problem.msg}}})
		return
	}
	status := 200
	if existingID == "" {
		status = 201
		f.next++
		if spec.intID {
			existingID = fmt.Sprint(f.next)
			rec["id"] = f.next
			if _, isCron := rec["expression"]; isCron {
				rec["ping_url"] = "https://fake.example.test/ping/" + existingID
			}
		} else {
			existingID = fmt.Sprintf("00000000-0000-4000-8000-%012d", f.next)
			rec["id"] = existingID
		}
	} else {
		rec["id"] = store[existingID]["id"]
		if p, ok := store[existingID]["ping_url"]; ok {
			rec["ping_url"] = p // never shown again after creation in the real API; kept server side
		}
	}
	rec["version"] = versionOf(rec, spec.ignored)
	store[existingID] = rec
	out := record{}
	for k, v := range rec {
		out[k] = v
	}
	if status == 200 {
		delete(out, "ping_url")
	}
	writeFake(w, status, map[string]any{"data": out})
}

func (f *fakeAPI) escalation(w http.ResponseWriter, r *http.Request, app string) {
	store := f.store(app, "escalation")
	cur := store["policy"]
	steps := []any{}
	if cur != nil {
		steps, _ = cur["steps"].([]any)
	}
	version := func(s []any) string { return versionOf(record{"steps": s}, nil) }
	switch r.Method {
	case "GET":
		writeFake(w, 200, map[string]any{"data": map[string]any{"steps": steps, "version": version(steps)}})
	case "PUT":
		var in record
		_ = json.NewDecoder(r.Body).Decode(&in)
		if v, _ := in["if_version"].(string); v != "" && v != version(steps) {
			writeFake(w, 409, map[string]any{"error": "conflict", "message": "This record changed since you read it. Read it again and retry with the new version."})
			return
		}
		next, _ := in["steps"].([]any)
		if next == nil {
			next = []any{}
		}
		if len(next) > 5 {
			writeFake(w, 422, map[string]any{"error": "validation_failed", "message": "At most 5 steps."})
			return
		}
		for _, s := range next {
			sm, _ := s.(map[string]any)
			if ids, _ := sm["channel_ids"].([]any); len(ids) == 0 {
				writeFake(w, 422, map[string]any{"error": "validation_failed", "message": "Each step needs a channel."})
				return
			}
		}
		store["policy"] = record{"steps": next}
		writeFake(w, 200, map[string]any{"data": map[string]any{"steps": next, "version": version(next)}})
	}
}

// mutate changes a stored record behind Terraform's back, as someone editing in the dashboard would.
func (f *fakeAPI) mutate(app, coll, id string, change func(record)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := f.store(app, coll)[id]
	change(rec)
	rec["version"] = versionOf(rec, fakeCollections[coll].ignored)
}

func (f *fakeAPI) remove(app, coll, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.store(app, coll), id)
}

func (f *fakeAPI) setEscalation(app string, steps []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store(app, "escalation")["policy"] = record{"steps": steps}
}

func (f *fakeAPI) idByName(app, coll, field, value string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, rec := range f.store(app, coll) {
		if fmt.Sprint(rec[field]) == value {
			return id
		}
	}
	return ""
}

func writeFake(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func problem(status int, code, msg string) *apiProblem { return &apiProblem{status, code, msg} }

func strField(in record, k string) (string, bool) { s, ok := in[k].(string); return s, ok && s != "" }

func tagList(in record) []any {
	tags := []any{}
	if raw, ok := in["tags"].([]any); ok {
		seen := map[string]bool{}
		for _, t := range raw {
			if s, ok := t.(string); ok && !seen[strings.ToLower(s)] {
				seen[strings.ToLower(s)] = true
				tags = append(tags, strings.ToLower(s))
			}
		}
	}
	return tags
}

func shapeCron(in record, created bool) (record, *apiProblem) {
	for _, k := range []string{"command", "expression", "timezone"} {
		if _, ok := strField(in, k); !ok {
			return nil, problem(422, "validation_failed", "The "+k+" field is required.")
		}
	}
	if len(strings.Fields(in["expression"].(string))) != 5 {
		return nil, problem(422, "validation_failed", "Enter a valid five-field cron expression.")
	}
	enabled, ok := in["enabled"].(bool)
	if !ok {
		enabled = true
	}
	return record{"command": in["command"], "expression": in["expression"], "timezone": in["timezone"],
		"grace_seconds": in["grace_seconds"], "max_runtime_seconds": in["max_runtime_seconds"],
		"enabled": enabled, "tags": tagList(in), "status": "unknown",
		"activated_at": time.Now().UTC().Format(time.RFC3339)}, nil
}

func shapeHeartbeat(in record, created bool) (record, *apiProblem) {
	if _, ok := strField(in, "name"); !ok {
		return nil, problem(422, "validation_failed", "The name field is required.")
	}
	if n, _ := in["interval_minutes"].(float64); n < 1 {
		return nil, problem(422, "validation_failed", "The interval minutes field must be at least 1.")
	}
	enabled, ok := in["enabled"].(bool)
	if !ok {
		enabled = true
	}
	return record{"name": in["name"], "interval_minutes": in["interval_minutes"], "grace_minutes": in["grace_minutes"],
		"enabled": enabled, "tags": tagList(in), "status": "unknown"}, nil
}

func shapeRule(in record, created bool) (record, *apiProblem) {
	switch in["kind"] {
	case "numeric_min", "numeric_max", "freshness":
	default:
		return nil, problem(422, "validation_failed", "The selected kind is invalid.")
	}
	out := record{"command": in["command"], "key": in["key"], "kind": in["kind"], "threshold": in["threshold"], "missing": "unknown"}
	if m, ok := strField(in, "missing"); ok {
		out["missing"] = m
	}
	for _, k := range []string{"job_name", "environment", "expression"} {
		if s, ok := strField(in, k); ok {
			out[k] = s
		}
	}
	return out, nil
}

func shapeWindow(in record, created bool) (record, *apiProblem) {
	if _, ok := strField(in, "name"); !ok {
		return nil, problem(422, "validation_failed", "The name field is required.")
	}
	out := record{"name": in["name"], "type": in["type"], "tags": tagList(in)}
	ids := []any{}
	if raw, ok := in["monitor_ids"].([]any); ok {
		ids = raw
	}
	out["monitor_ids"] = ids
	switch in["type"] {
	case "once":
		starts, e1 := time.Parse(time.RFC3339, fmt.Sprint(in["starts_at"]))
		ends, e2 := time.Parse(time.RFC3339, fmt.Sprint(in["ends_at"]))
		if e1 != nil || e2 != nil {
			return nil, problem(422, "validation_failed", "A one-off window needs starts_at and ends_at.")
		}
		if !ends.After(starts) {
			return nil, problem(422, "validation_failed", "The window must end after it starts.")
		}
		out["starts_at"] = starts.UTC().Format("2006-01-02T15:04:05+00:00")
		out["ends_at"] = ends.UTC().Format("2006-01-02T15:04:05+00:00")
	case "weekly":
		days := []int{}
		for _, d := range in["days"].([]any) {
			days = append(days, int(d.(float64)))
		}
		sort.Ints(days)
		out["days"], out["start_time"], out["duration_minutes"] = days, in["start_time"], in["duration_minutes"]
	default:
		return nil, problem(422, "validation_failed", "The selected type is invalid.")
	}
	return out, nil
}
