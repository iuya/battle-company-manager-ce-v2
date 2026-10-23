package core

import (
	"context"
	"errors"
	"testing"
)

type countingCompanies struct {
	companyRepository
	reads int
	err   error
}

func (c *countingCompanies) Read(_ context.Context, campaignID, companyID int64) (CompanyState, error) {
	c.reads++
	if c.err != nil {
		return CompanyState{}, c.err
	}
	return CompanyState{Company: CampaignCompany{ID: companyID, Name: "x"}}, nil
}

func TestReadScopeReadsOnceInAScopeAndAlwaysWithout(t *testing.T) {
	inner := &countingCompanies{}
	r := ScopeCompanies(inner)
	plain := context.Background()
	for range 3 {
		if _, err := r.Read(plain, 1, 2); err != nil {
			t.Fatal(err)
		}
	}
	if inner.reads != 3 {
		t.Fatalf("without a scope every read goes through: %d", inner.reads)
	}
	inner.reads = 0
	ctx := WithReadScope(plain)
	if WithReadScope(ctx) != ctx {
		t.Fatal("a second scope must not replace the first")
	}
	for range 3 {
		st, err := r.Read(ctx, 1, 2)
		if err != nil || st.Company.ID != 2 {
			t.Fatal(st, err)
		}
	}
	if _, err := r.Read(ctx, 1, 3); err != nil { // another company is another read
		t.Fatal(err)
	}
	if inner.reads != 2 {
		t.Fatalf("one read per company in a scope: %d", inner.reads)
	}
	// a new scope sees fresh data
	if _, err := r.Read(WithReadScope(plain), 1, 2); err != nil || inner.reads != 3 {
		t.Fatal(err, inner.reads)
	}
}

func TestReadScopeDoesNotRememberErrors(t *testing.T) {
	inner := &countingCompanies{err: ErrNotFound}
	r := ScopeCompanies(inner)
	ctx := WithReadScope(context.Background())
	for range 2 {
		if _, err := r.Read(ctx, 1, 2); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if inner.reads != 2 {
		t.Fatalf("a failed read is tried again: %d", inner.reads)
	}
}
