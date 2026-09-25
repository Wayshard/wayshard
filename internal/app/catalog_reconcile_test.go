package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/storage"
)

func openAuditStore(t *testing.T) *storage.Store {
	t.Helper()
	st, err := storage.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func loadAuditCatalog(t *testing.T, userText string) *harness.Catalog {
	t.Helper()
	p := filepath.Join(t.TempDir(), "harnesses.toml")
	if err := os.WriteFile(p, []byte(userText), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := harness.LoadCatalog(p)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	return cat
}

func readyRow(id, defID, exe, fingerprint string) domain.HarnessInstallation {
	return domain.HarnessInstallation{
		ID: id, DefinitionID: defID, DisplayName: defID, Executable: exe, Adapter: "generic",
		Health: domain.HarnessReady, Compatibility: domain.CompatEnhanced,
		AuthStatus:            "unknown",
		DefinitionFingerprint: fingerprint,
	}
}

func candidateFor(t *testing.T, sc *storeCandidates, defID string) (domain.HarnessInstallation, bool) {
	t.Helper()
	cands, err := sc.Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.Harness.DefinitionID == defID {
			return c.Harness, true
		}
	}
	return domain.HarnessInstallation{}, false
}

func TestReconcileDisabledDefinitionNotRoutable(t *testing.T) {
	ctx := context.Background()
	st := openAuditStore(t)
	shipped := harness.ShippedCatalog()
	oc, _ := shipped.ByID("opencode")
	if err := st.UpsertHarnessInstallation(ctx, ptrRow(readyRow("i1", "opencode", "/tmp/oc", oc.ExecutionFingerprint()))); err != nil {
		t.Fatal(err)
	}
	disabled := loadAuditCatalog(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\nenabled=false\n")
	sc := &storeCandidates{Store: st, Catalog: disabled}
	if _, ok := candidateFor(t, sc, "opencode"); ok {
		t.Fatal("disabled definition still produced a routable candidate")
	}
}

func TestReconcileChangedDefinitionNotRoutable(t *testing.T) {
	ctx := context.Background()
	st := openAuditStore(t)
	shipped := harness.ShippedCatalog()
	oc, _ := shipped.ByID("opencode")
	if err := st.UpsertHarnessInstallation(ctx, ptrRow(readyRow("i1", "opencode", "/tmp/oc", oc.ExecutionFingerprint()))); err != nil {
		t.Fatal(err)
	}
	changed := loadAuditCatalog(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\nexecutables=[\"totally-other\"]\n")
	sc := &storeCandidates{Store: st, Catalog: changed}
	if _, ok := candidateFor(t, sc, "opencode"); ok {
		t.Fatal("materially changed definition kept the stale installation routable")
	}
}

func TestReconcileRemovedCustomDefinitionNotRoutable(t *testing.T) {
	ctx := context.Background()
	st := openAuditStore(t)
	withCustom := loadAuditCatalog(t, "schema_version=1\n[[harness]]\nid=\"my-harness\"\nexecutables=[\"my-harness\"]\nacp=\"native\"\n")
	d, ok := withCustom.ByID("my-harness")
	if !ok {
		t.Fatal("custom definition missing")
	}
	if err := st.UpsertHarnessInstallation(ctx, ptrRow(readyRow("i1", "my-harness", "/tmp/my", d.ExecutionFingerprint()))); err != nil {
		t.Fatal(err)
	}
	if _, ok := candidateFor(t, &storeCandidates{Store: st, Catalog: withCustom}, "my-harness"); !ok {
		t.Fatal("custom definition should be routable while present")
	}
	if _, ok := candidateFor(t, &storeCandidates{Store: st, Catalog: harness.ShippedCatalog()}, "my-harness"); ok {
		t.Fatal("removed custom definition kept the stale installation routable")
	}
}

func TestReplaceHarnessInstallationsClearsStale(t *testing.T) {
	ctx := context.Background()
	st := openAuditStore(t)
	shipped := harness.ShippedCatalog()
	oc, _ := shipped.ByID("opencode")
	if err := st.UpsertHarnessInstallation(ctx, ptrRow(readyRow("i1", "opencode", "/tmp/oc", oc.ExecutionFingerprint()))); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceHarnessInstallations(ctx, nil); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListHarnessInstallations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("replace left %d stale installations", len(list))
	}
}

func TestLoadHarnessCatalogSurfacesMalformedUserFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "harnesses.toml")
	if err := os.WriteFile(p, []byte("schema_version = 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := loadHarnessCatalog(nil, p)
	found := false
	for _, d := range cat.Diagnostics {
		if d.Source == "user" && strings.Contains(d.Message, "rejected") {
			found = true
		}
	}
	if !found {
		t.Fatalf("malformed user catalog diagnostic not surfaced: %v", cat.Diagnostics)
	}
}

func ptrRow(h domain.HarnessInstallation) *domain.HarnessInstallation { return &h }
