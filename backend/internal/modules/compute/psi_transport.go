package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
)

// --- PSI multi-node transport (Direction D, the deployment half) ---
//
// runTwoRoundPSI (psi_tworound.go) verified the protocol with in-process party
// agents. This wires those agents to a real network boundary: each party runs as
// a long-lived HTTP server holding its set + secret in memory, and the
// orchestrator drives the two rounds over the wire, exchanging ONLY blinded group
// elements (hex) — never plaintext. This is the shape a real multi-operator
// deployment takes; here the servers run on localhost for verification.
//
// 诚实边界: localhost transport with no auth/TLS. Production puts each party
// server on a separate operator's host behind mTLS + authn, and the threat model
// stays semi-honest (no malicious-party defence). The protocol + wire format are
// real and tested; what remains is ops (separate hosts, transport security), not
// cryptography.

// psiPointsWire is the JSON wire format for a list of blinded group elements,
// encoded as base-16 strings.
type psiPointsWire struct {
	Points []string `json:"points"`
}

func encodePSIPoints(pts []*big.Int) psiPointsWire {
	out := make([]string, len(pts))
	for i, p := range pts {
		out[i] = p.Text(16)
	}
	return psiPointsWire{Points: out}
}

func decodePSIPoints(w psiPointsWire) ([]*big.Int, error) {
	out := make([]*big.Int, len(w.Points))
	for i, s := range w.Points {
		n, ok := new(big.Int).SetString(s, 16)
		if !ok {
			return nil, fmt.Errorf("compute: bad psi point %q", s)
		}
		out[i] = n
	}
	return out, nil
}

// PSIPartyServer is a long-lived party node: it holds its set + secret in memory
// and exposes only blinded-point operations over HTTP.
type PSIPartyServer struct {
	agent *psiPartyAgent
}

// NewPSIPartyServer creates a party server holding the given set with a fresh secret.
func NewPSIPartyServer(set []string) (*PSIPartyServer, error) {
	a, err := newPSIPartyAgent(set)
	if err != nil {
		return nil, err
	}
	return &PSIPartyServer{agent: a}, nil
}

// Handler exposes /round1 (blind own set), /reblind (raise peers' points by own
// secret), and /label (return THIS party's own elements at matched indices — used
// only by the result party). The raw set is never returned wholesale.
func (s *PSIPartyServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/round1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, encodePSIPoints(s.agent.round1()))
	})
	mux.HandleFunc("/reblind", func(w http.ResponseWriter, r *http.Request) {
		var in psiPointsWire
		if err := readJSON(r, &in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		pts, err := decodePSIPoints(in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, encodePSIPoints(s.agent.reblind(pts)))
	})
	mux.HandleFunc("/label", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Indices []int `json:"indices"`
		}
		if err := readJSON(r, &in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, struct {
			Elements []string `json:"elements"`
		}{Elements: s.agent.elementsAt(in.Indices)})
	})
	return mux
}

// httpPSIParty is the orchestrator's client for one remote party server.
type httpPSIParty struct {
	baseURL string
	client  *http.Client
}

func (c httpPSIParty) round1(ctx context.Context) ([]*big.Int, error) {
	var out psiPointsWire
	if err := c.post(ctx, "/round1", struct{}{}, &out); err != nil {
		return nil, err
	}
	return decodePSIPoints(out)
}

func (c httpPSIParty) reblind(ctx context.Context, pts []*big.Int) ([]*big.Int, error) {
	var out psiPointsWire
	if err := c.post(ctx, "/reblind", encodePSIPoints(pts), &out); err != nil {
		return nil, err
	}
	return decodePSIPoints(out)
}

func (c httpPSIParty) label(ctx context.Context, idx []int) ([]string, error) {
	var out struct {
		Elements []string `json:"elements"`
	}
	if err := c.post(ctx, "/label", struct {
		Indices []int `json:"indices"`
	}{Indices: idx}, &out); err != nil {
		return nil, err
	}
	return out.Elements, nil
}

func (c httpPSIParty) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("compute: psi party %s%s: %s: %s", c.baseURL, path, resp.Status, bytes.TrimSpace(msg))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// RunHTTPPSI executes the two-round protocol across remote party servers. The
// orchestrator only ever sends/receives blinded points (and matched indices);
// parties[0] is the result party and labels the intersection from its own set.
func RunHTTPPSI(ctx context.Context, parties []httpPSIParty) (PSIResult, error) {
	if len(parties) < 2 {
		return PSIResult{}, ErrMPCParties
	}
	// Round 1: each party blinds its own set.
	blinded := make([][]*big.Int, len(parties))
	for i, p := range parties {
		pts, err := p.round1(ctx)
		if err != nil {
			return PSIResult{}, err
		}
		blinded[i] = pts
	}
	// Round 2: route each party's points through every OTHER party to raise them
	// by the product of all secrets.
	fully := make([][]*big.Int, len(parties))
	for i := range parties {
		pts := blinded[i]
		for j := range parties {
			if j == i {
				continue
			}
			rb, err := parties[j].reblind(ctx, pts)
			if err != nil {
				return PSIResult{}, err
			}
			pts = rb
		}
		fully[i] = pts
	}

	key := func(x *big.Int) string { return x.Text(16) }
	otherSets := make([]map[string]struct{}, 0, len(parties)-1)
	for i := 1; i < len(parties); i++ {
		m := make(map[string]struct{}, len(fully[i]))
		for _, p := range fully[i] {
			m[key(p)] = struct{}{}
		}
		otherSets = append(otherSets, m)
	}
	seen := make(map[string]struct{})
	matchIdx := make([]int, 0)
	for idx := range fully[0] {
		k := key(fully[0][idx])
		inAll := true
		for _, m := range otherSets {
			if _, ok := m[k]; !ok {
				inAll = false
				break
			}
		}
		if !inAll {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		matchIdx = append(matchIdx, idx)
	}

	inter, err := parties[0].label(ctx, matchIdx)
	if err != nil {
		return PSIResult{}, err
	}
	sort.Strings(inter)
	return PSIResult{Intersection: inter, Cardinality: len(inter)}, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
