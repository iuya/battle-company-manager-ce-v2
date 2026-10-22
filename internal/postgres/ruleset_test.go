package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func testRuleset(kind core.RulesetKind, name, version, body string) core.StoredRuleset {
	sum := sha256.Sum256([]byte(body))
	return core.StoredRuleset{
		RulesetInfo: core.RulesetInfo{Kind: kind, Hash: hex.EncodeToString(sum[:]), Name: name, Version: version},
		Size:        len(body),
		Content:     []byte(body),
	}
}

func TestRulesets(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	repo := s.Rulesets()

	u, err := s.CreateUser(ctx, fmt.Sprintf("rs-%d@example.com", time.Now().UnixNano()), "Uploader", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tag := fmt.Sprint(time.Now().UnixNano())
	name := "Test edition " + tag

	a := testRuleset(core.KindVanilla, name, "1", `{"a":"`+tag+`"}`)
	b := testRuleset(core.KindVanilla, name, "2", `{"b":"`+tag+`"}`)
	b.UploadedBy = u.ID
	for _, r := range []core.StoredRuleset{a, a, b} { // the double insert is a no-op
		if err := repo.Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.Get(ctx, b.Hash)
	if err != nil || !bytes.Equal(got.Content, b.Content) || got.Size != b.Size || got.UploadedBy != u.ID || got.Kind != core.KindVanilla || got.Default {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got, err = repo.Get(ctx, a.Hash); err != nil || got.UploadedBy != 0 {
		t.Fatalf("shipped file must have no uploader: %+v, %v", got, err)
	}
	if info, err := repo.Info(ctx, a.Hash); err != nil || info != a.RulesetInfo {
		t.Fatalf("got %+v, %v", info, err)
	}
	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := repo.Info(ctx, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v", err)
	}

	// Files are identified by their hash alone: the same name and version with other content is another file.
	twin := testRuleset(core.KindVanilla, name, "2", `{"b":"`+tag+`","twin":true}`)
	if err := repo.Insert(ctx, twin); err != nil {
		t.Fatalf("same name and version, other content: %v", err)
	}
	if info, err := repo.Info(ctx, twin.Hash); err != nil || info.Name != name || info.Version != "2" || info.ShortHash() == b.ShortHash() {
		t.Fatalf("twin: %+v, %v", info, err)
	}
	if info, err := repo.Info(ctx, b.Hash); err != nil || info.Hash != b.Hash {
		t.Fatalf("the first file is untouched: %+v, %v", info, err)
	}

	// Default switch; restore whatever was default so a shared database is left as found.
	prev, prevErr := repo.Default(ctx, core.KindVanilla)
	defer func() {
		if prevErr == nil {
			_ = repo.SetDefault(ctx, core.KindVanilla, prev.Hash)
		}
	}()
	if err := repo.SetDefault(ctx, core.KindVanilla, a.Hash); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDefault(ctx, core.KindVanilla, b.Hash); err != nil {
		t.Fatal(err)
	}
	if d, err := repo.Default(ctx, core.KindVanilla); err != nil || d.Hash != b.Hash || !d.Default {
		t.Fatalf("got %+v, %v", d, err)
	}
	if info, _ := repo.Info(ctx, a.Hash); info.Default {
		t.Fatal("the previous default must be unmarked")
	}
	if err := repo.SetDefault(ctx, core.KindVanilla, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if err := repo.SetDefault(ctx, core.KindGlossary, a.Hash); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a file cannot become the default of another kind, got %v", err)
	}
	if d, _ := repo.Default(ctx, core.KindVanilla); d.Hash != b.Hash {
		t.Fatal("a failed switch must leave the default alone")
	}

	// The partial unique index is the last line of defence.
	if _, err := s.pool.Exec(ctx, `UPDATE rulesets SET is_default = true WHERE hash = $1`, a.Hash); err == nil {
		t.Fatal("two defaults of one kind must be refused")
	}
	if _, err := repo.Default(ctx, "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}
