package core

import (
	"errors"
	"strings"
	"testing"
)

func TestGrantInputValidate(t *testing.T) {
	ok, err := GrantInput{Gold: -5, Influence: 2, Note: "  Narrative scenario  ", UnitID: 9}.Validate()
	if err != nil || ok.Note != "Narrative scenario" || ok.UnitID != 0 {
		t.Errorf("valid: %+v %v", ok, err)
	}
	if ok, err := (GrantInput{Experience: 2, UnitID: 4, Note: "x"}).Validate(); err != nil || ok.UnitID != 4 {
		t.Errorf("experience with a unit: %+v %v", ok, err)
	}
	for name, in := range map[string]GrantInput{
		"no note":         {Gold: 1},
		"long note":       {Gold: 1, Note: strings.Repeat("a", 201)},
		"nothing given":   {Note: "x"},
		"xp without unit": {Experience: 1, Note: "x"},
		"too much gold":   {Gold: 1001, Note: "x"},
	} {
		if _, err := in.Validate(); !errors.Is(err, ErrInvalidCompany) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCanGrant(t *testing.T) {
	d := CampaignDetail{OrganiserID: 5}
	if err := d.CanGrant(User{ID: 5}); err != nil {
		t.Errorf("organiser: %v", err)
	}
	if err := d.CanGrant(User{ID: 6, Role: RoleAdmin}); err != nil {
		t.Errorf("admin: %v", err)
	}
	if err := d.CanGrant(User{ID: 6}); !errors.Is(err, ErrRefused) {
		t.Errorf("player: %v", err)
	}
	if err := (CampaignDetail{}).CanGrant(User{ID: 0}); !errors.Is(err, ErrRefused) {
		t.Errorf("no organiser id must not match user 0: %v", err)
	}
	d.Status = CampaignFinished
	if err := d.CanGrant(User{ID: 5}); !errors.Is(err, ErrRefused) {
		t.Errorf("finished: %v", err)
	}
}
