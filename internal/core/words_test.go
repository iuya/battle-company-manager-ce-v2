package core

import "testing"

func TestWords(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{Plural(1, "casualty", "casualties"), "casualty"},
		{Plural(0, "casualty", "casualties"), "casualties"},
		{Plural(2, "", "s"), "s"},
		{Plural(1, "", "s"), ""},
		{Count(0, "Sergeant"), "no Sergeants"},
		{Count(1, "Sergeant"), "one Sergeant"},
		{Count(2, "leader"), "two leaders"},
		{Count(6, "Sergeant"), "six Sergeants"},
		{Count(7, "Sergeant"), "7 Sergeants"},
		{Count(-1, "Sergeant"), "-1 Sergeants"},
		{Inches(6), "6"},
		{Inches(4.5), "4.5"},
		{Inches(0), "0"},
		{Capitalise(""), ""},
		{Capitalise("fight"), "Fight"},
		{Capitalise("Fight value"), "Fight value"},
		{Capitalise(" x"), " x"},
		{LowerFirst(""), ""},
		{LowerFirst("Not allowed"), "not allowed"},
		{LowerFirst("ALL"), "aLL"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}
