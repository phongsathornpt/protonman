package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

const (
	pinnedSchemaPath = "testdata/acp-schema-v" + SchemaArtifactVersion + ".json"
	pinnedMetaPath   = "testdata/acp-meta-v" + SchemaArtifactVersion + ".json"
	schemaResource   = "acp://schema/v1.json"
)

var (
	schemaOnce     sync.Once
	schemaCompiler *jsonschema.Compiler
	schemaErr      error
)

func loadPinnedSchemaCompiler(t testing.TB) *jsonschema.Compiler {
	t.Helper()
	schemaOnce.Do(func() {
		raw, err := os.ReadFile(filepath.FromSlash(pinnedSchemaPath))
		if err != nil {
			schemaErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			schemaErr = err
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(schemaResource, doc); err != nil {
			schemaErr = err
			return
		}
		schemaCompiler = compiler
	})
	if schemaErr != nil {
		t.Fatalf("load pinned ACP schema %s: %v", pinnedSchemaPath, schemaErr)
	}
	return schemaCompiler
}

// validateACPSchema validates a wire value against the named definition of the
// vendored ACP schema artifact. The value is round-tripped through JSON so the
// check observes exactly what an ACP peer would receive.
func validateACPSchema(t testing.TB, definition string, value any) error {
	t.Helper()
	compiler := loadPinnedSchemaCompiler(t)
	compiled, err := compiler.Compile(schemaResource + "#/$defs/" + definition)
	if err != nil {
		t.Fatalf("compile ACP schema definition %s: %v", definition, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", definition, err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode %s payload: %v", definition, err)
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("%s payload violates ACP schema %s: %w\npayload: %s", definition, SchemaArtifactVersion, err, encoded)
	}
	return nil
}

func assertACPSchema(t testing.TB, definition string, value any) {
	t.Helper()
	if err := validateACPSchema(t, definition, value); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaHarnessRejectsInvalidPayload(t *testing.T) {
	if err := validateACPSchema(t, "NewSessionResponse", map[string]any{"modes": "not-an-object"}); err == nil {
		t.Fatal("schema harness accepted a payload missing sessionId with malformed modes")
	}
}

type pinnedMeta struct {
	Version         int               `json:"version"`
	AgentMethods    map[string]string `json:"agentMethods"`
	ClientMethods   map[string]string `json:"clientMethods"`
	ProtocolMethods map[string]string `json:"protocolMethods"`
}

func loadPinnedMeta(t testing.TB) pinnedMeta {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(pinnedMetaPath))
	if err != nil {
		t.Fatalf("read pinned ACP meta: %v", err)
	}
	var meta pinnedMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("decode pinned ACP meta: %v", err)
	}
	return meta
}

func TestPinnedSchemaMatchesProtocolVersion(t *testing.T) {
	meta := loadPinnedMeta(t)
	if meta.Version != ProtocolVersion {
		t.Fatalf("pinned schema wire version = %d, ProtocolVersion = %d", meta.Version, ProtocolVersion)
	}
}

// TestStableV1CoverageMethodsExistInPinnedSchema keeps the manifest honest: every
// method-shaped feature name must be a method the pinned ACP artifact defines.
func TestStableV1CoverageMethodsExistInPinnedSchema(t *testing.T) {
	meta := loadPinnedMeta(t)
	known := map[string]bool{}
	for _, group := range []map[string]string{meta.AgentMethods, meta.ClientMethods, meta.ProtocolMethods} {
		for _, method := range group {
			known[method] = true
		}
	}
	for _, feature := range StableV1Coverage {
		if feature.Method == "" {
			continue
		}
		if !known[feature.Method] {
			t.Errorf("manifest feature %q references method %q absent from pinned ACP %s", feature.Name, feature.Method, SchemaArtifactVersion)
		}
	}
}

// TestAdvertisedCapabilitiesMatchManifest asserts the negotiated `initialize`
// capabilities never promise anything the manifest marks as unsupported, and
// that every advertised capability-bearing feature is on the wire.
func TestAdvertisedCapabilitiesMatchManifest(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	WithSessionRegistryFactory(nil)(server)
	result, _, err := server.dispatch(context.Background(), RPCRequest{Method: "initialize"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	assertACPSchema(t, "InitializeResponse", result)

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		AgentCapabilities struct {
			LoadSession         bool                       `json:"loadSession"`
			SessionCapabilities map[string]json.RawMessage `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
		AuthMethods []json.RawMessage `json:"authMethods"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}

	capabilityFeatures := map[string]string{
		"session/list":   "list",
		"session/resume": "resume",
		"session/delete": "delete",
		"session/close":  "close",
	}
	for featureName, capability := range capabilityFeatures {
		feature, ok := stableFeature(featureName)
		if !ok {
			t.Fatalf("manifest is missing %q", featureName)
		}
		_, onWire := wire.AgentCapabilities.SessionCapabilities[capability]
		if feature.Advertised != onWire {
			t.Errorf("sessionCapabilities.%s on wire = %v, manifest Advertised = %v", capability, onWire, feature.Advertised)
		}
	}
	if feature, _ := stableFeature("session/load"); feature.Advertised != wire.AgentCapabilities.LoadSession {
		t.Errorf("loadSession on wire = %v, manifest Advertised = %v", wire.AgentCapabilities.LoadSession, feature.Advertised)
	}
	if feature, _ := stableFeature("authentication"); !feature.Advertised && len(wire.AuthMethods) != 0 {
		t.Errorf("authMethods advertised while authentication is unsupported: %s", payload)
	}
	if strings.Contains(string(payload), `"elicitation"`) {
		// Elicitation is a client capability; the agent never advertises it.
		t.Errorf("agent initialize response must not contain client-only elicitation capability: %s", payload)
	}
}

func TestSessionNewResponseMatchesPinnedSchema(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	result, notify, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: mustJSON(t, map[string]any{"cwd": t.TempDir()}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	assertACPSchema(t, "NewSessionResponse", result)
	if notify == nil {
		t.Fatal("session/new did not announce available commands")
	}
	assertACPSchema(t, "SessionNotification", notify.Params)
}
