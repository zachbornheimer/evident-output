package evo_test

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestEncodeEnvelope_WrapsPayload(t *testing.T) {
	t.Parallel()
	api := evo.NewAPI(evo.APIConfig{
		APIDefaults: true,
		Livemode:    true,
		RequestID:   "req_testfixed",
	})
	body, err := api.EncodeEnvelope("health", map[string]any{"healthy": true, "timestamp": "2026-09-07T00:00:00Z"})
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	for _, key := range []string{"object", "livemode", "request_id", "data"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("envelope missing %q: %s", key, body)
		}
	}
	if got["object"] != "health" {
		t.Fatalf("object = %v, want health", got["object"])
	}
	if got["livemode"] != true {
		t.Fatalf("livemode = %v, want true", got["livemode"])
	}
	if got["request_id"] != "req_testfixed" {
		t.Fatalf("request_id = %v, want req_testfixed", got["request_id"])
	}
	if _, ok := got["idempotency_key"]; ok {
		t.Fatalf("idempotency_key present without caller set: %s", body)
	}
	data, ok := got["data"].(map[string]any)
	if !ok || data["healthy"] != true {
		t.Fatalf("data = %#v, want anonymizer-shaped health payload", got["data"])
	}
}

func TestAPIDefaults_False_SkipsInjection(t *testing.T) {
	t.Parallel()
	api := evo.NewAPI(evo.APIConfig{APIDefaults: false})
	body, err := api.EncodeEnvelope("health", map[string]string{"healthy": "yes"})
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["livemode"] != false {
		t.Fatalf("livemode = %v, want false when defaults off and unset", got["livemode"])
	}
	if got["request_id"] != "" {
		t.Fatalf("request_id = %v, want empty when defaults off", got["request_id"])
	}
}

func TestEncodeEnvelope_RequiresObject(t *testing.T) {
	t.Parallel()
	_, err := evo.NewAPI().EncodeEnvelope("", map[string]int{"n": 1})
	if err == nil || !strings.Contains(err.Error(), "object type is required") {
		t.Fatalf("err = %v, want object type is required", err)
	}
}

func TestEncodeProblem_RFC7807(t *testing.T) {
	t.Parallel()
	api := evo.NewAPI(evo.APIConfig{
		APIDefaults: true,
		Livemode:    true,
		RequestID:   "req_problem",
	})
	body, err := api.EncodeProblem(evo.ProblemDetails{
		Type:   "https://api.example.com/errors/validation-error",
		Title:  "Validation Error",
		Status: 422,
		Code:   "validation_error",
		Detail: "missing file",
		Param:  "/file",
	})
	if err != nil {
		t.Fatalf("EncodeProblem: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if got["type"] != "https://api.example.com/errors/validation-error" {
		t.Fatalf("type = %v", got["type"])
	}
	if got["title"] != "Validation Error" {
		t.Fatalf("title = %v", got["title"])
	}
	if got["status"] != float64(422) {
		t.Fatalf("status = %v, want 422", got["status"])
	}
	if got["code"] != "validation_error" {
		t.Fatalf("code = %v", got["code"])
	}
	if got["livemode"] != true {
		t.Fatalf("livemode = %v, want true", got["livemode"])
	}
	if got["request_id"] != "req_problem" {
		t.Fatalf("request_id = %v", got["request_id"])
	}
	if got["detail"] != "missing file" {
		t.Fatalf("detail = %v", got["detail"])
	}
}

func TestInit_JSONDocument_NotEnvelope(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{
		Isolated:   true,
		Projection: evo.ProjectionJSON,
		Options:    []evo.Option{evo.To(io.Discard)},
	})
	out.Task("health").Done()
	_ = out.Finish()
	body, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["livemode"]; ok {
		t.Fatalf("CLI JSONDocument grew livemode wrapper: %s", body)
	}
	if _, ok := got["request_id"]; ok {
		t.Fatalf("CLI JSONDocument grew request_id wrapper: %s", body)
	}
	if _, ok := got["schema_version"]; !ok {
		t.Fatalf("CLI JSONDocument missing schema_version: %s", body)
	}
}

func TestAPI_MintsRequestID(t *testing.T) {
	t.Parallel()
	body, err := evo.NewAPI().EncodeEnvelope("quote_result", map[string]string{"state": "TX"})
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	id, _ := got["request_id"].(string)
	if !strings.HasPrefix(id, "req_") || len(id) <= 4 {
		t.Fatalf("request_id = %q, want req_ prefix with entropy", id)
	}
}
