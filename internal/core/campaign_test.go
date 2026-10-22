package core

import (
	"errors"
	"strings"
	"testing"
)

func TestCampaignInputValidate(t *testing.T) {
	in, err := CampaignInput{Name: "  Siege  ", Description: " x "}.Validate()
	if err != nil || in.Name != "Siege" || in.Description != "x" {
		t.Errorf("valid input: got %+v, %v", in, err)
	}
	for name, in := range map[string]CampaignInput{
		"empty name":       {Name: " "},
		"long name":        {Name: strings.Repeat("a", 61)},
		"long description": {Name: "ok", Description: strings.Repeat("a", 281)},
	} {
		if _, err := in.Validate(); !errors.Is(err, ErrInvalidCampaign) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
