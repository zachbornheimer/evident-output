package evo

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	requestIDPrefix      = "req_"
	requestIDRandomBytes = 10
)

// readRandom is the facade over crypto/rand.Read so tests can inject entropy.
var readRandom = rand.Read

// API encodes HTTP JSON with the V1 envelope zyins ships: object, livemode,
// request_id, data. It is not an Output and does not wrap CLI JSONDocument.
//
// NewAPI() turns APIDefaults on (inject livemode and request_id when omitted).
// NewAPI(APIConfig{APIDefaults: false}) encodes exactly the caller's fields.
type API struct {
	defaults  bool
	livemode  bool
	requestID string
	newID     func() (string, error)
}

// APIConfig configures NewAPI. CLI Init never reads this type.
type APIConfig struct {
	// APIDefaults injects livemode and a minted request_id when omitted.
	// NewAPI() with no config turns this on; a passed config uses the
	// field as written (false skips injection).
	APIDefaults bool
	Livemode    bool
	RequestID   string
}

// Envelope is the V1 success object. IdempotencyKey is optional; zyins does
// not echo it on the wire.
type Envelope struct {
	Object         string `json:"object"`
	Livemode       bool   `json:"livemode"`
	RequestID      string `json:"request_id"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Data           any    `json:"data"`
}

// ProblemDetails is an RFC 7807 problem plus livemode (zyins
// WriteProblemEnriched). Named apart from CLI Problem.
type ProblemDetails struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Param     string `json:"param,omitempty"`
	DocURL    string `json:"doc_url,omitempty"`
	Livemode  bool   `json:"livemode"`
}

// NewAPI returns an HTTP JSON encoder. With no config, APIDefaults is on,
// livemode defaults to true, and request_id is minted per Encode call.
// Passing a config uses the struct fields as written — including
// APIDefaults: false, which skips injection.
func NewAPI(configs ...APIConfig) *API {
	a := &API{defaults: true, livemode: true, newID: generatedRequestID}
	if len(configs) == 0 {
		return a
	}
	cfg := configs[0]
	a.defaults = cfg.APIDefaults
	a.livemode = cfg.Livemode
	a.requestID = cfg.RequestID
	return a
}

// EncodeEnvelope wraps data as a V1 success envelope. object is required.
func (a *API) EncodeEnvelope(object string, data any) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("encode envelope: API is nil")
	}
	if object == "" {
		return nil, fmt.Errorf("encode envelope: object type is required")
	}
	env := Envelope{Object: object, Data: data, Livemode: a.livemode, RequestID: a.requestID}
	if a.defaults {
		if env.RequestID == "" {
			id, err := a.newID()
			if err != nil {
				return nil, fmt.Errorf("encode envelope: mint request_id: %w", err)
			}
			env.RequestID = id
		}
	}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("encode envelope %q: %w", object, err)
	}
	return body, nil
}

// EncodeProblem encodes an RFC 7807 problem. APIDefaults fills livemode and
// request_id when omitted.
func (a *API) EncodeProblem(p ProblemDetails) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("encode problem: API is nil")
	}
	if a.defaults {
		p.Livemode = a.livemode
		if p.RequestID == "" {
			if a.requestID != "" {
				p.RequestID = a.requestID
			} else {
				id, err := a.newID()
				if err != nil {
					return nil, fmt.Errorf("encode problem: mint request_id: %w", err)
				}
				p.RequestID = id
			}
		}
	}
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode problem: %w", err)
	}
	return body, nil
}

func generatedRequestID() (string, error) {
	var b [requestIDRandomBytes]byte
	if _, err := readRandom(b[:]); err != nil {
		return "", fmt.Errorf("read entropy: %w", err)
	}
	return requestIDPrefix + hex.EncodeToString(b[:]), nil
}
