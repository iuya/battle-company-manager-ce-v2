package lean

import (
	"os"
	"strings"
	"testing"
)

func TestPercentShareIsExact(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Share
	}{
		{"50", Share{1, 2}},
		{"33.33", Share{3333, 10000}},
		{"66", Share{33, 50}},
		{"33", Share{33, 100}},
		{"100", Share{1, 1}},
		{"0.01", Share{1, 10000}},
		{"12.5", Share{1, 8}},
		{"25.0", Share{1, 4}},
	} {
		got, err := PercentShare(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("PercentShare(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"-1", "100.5", "abc", "", "33.3333333333"} {
		if got, err := PercentShare(bad); err == nil {
			t.Errorf("PercentShare(%q) = %v, want an error", bad, got)
		}
	}
}

func TestParseShare(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    Share
		wantErr string
	}{
		{"1/3", Share{1, 3}, ""},
		{" 2 / 4 ", Share{1, 2}, ""},
		{"0/5", Share{0, 1}, ""},
		{"3/3", Share{1, 1}, ""},
		{"1/0", Share{}, "denominator"},
		{"-1/3", Share{}, "whole numbers"},
		{"4/3", Share{}, "more than the whole"},
		{"1/3000000", Share{}, "above 1000000"},
		{"third", Share{}, "not a fraction"},
		{"1/x", Share{}, "whole numbers"},
	} {
		got, err := ParseShare(tt.in)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseShare(%q) = %v, %v; want an error with %q", tt.in, got, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseShare(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestShareOfMatchesAPlainIntegerReference(t *testing.T) {
	for _, s := range []Share{{1, 3}, {1, 2}, {2, 3}, {3, 4}} {
		for n := 1; n <= 100; n++ {
			up, down := 0, 0
			for k := 0; ; k++ { // the smallest k with k*den >= n*num, the largest with k*den <= n*num
				if k*s.Den >= n*s.Num {
					up = k
					break
				}
			}
			for k := n; k >= 0; k-- {
				if k*s.Den <= n*s.Num {
					down = k
					break
				}
			}
			if got := s.Of(n, true); got != up {
				t.Errorf("%v of %d rounded up = %d, want %d", s, n, got, up)
			}
			if got := s.Of(n, false); got != down {
				t.Errorf("%v of %d rounded down = %d, want %d", s, n, got, down)
			}
		}
	}
}

func TestShareNearMiss(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string // the fraction, "" for none
	}{
		{"33.33", "1/3"}, {"66.67", "2/3"}, {"33.3", "1/3"}, {"16.67", "1/6"},
		{"33", ""}, {"66", ""}, {"50", ""}, {"25", ""}, {"12.5", ""}, {"40", ""}, {"100", ""},
	} {
		s, err := PercentShare(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := s.NearMiss()
		if tt.want == "" && ok || tt.want != "" && got.String() != tt.want {
			t.Errorf("NearMiss(%s) = %v, %v; want %q", tt.in, got, ok, tt.want)
		}
	}
	if _, ok := (Share{1, 3}).NearMiss(); ok {
		t.Error("1/3 is exact, not a near miss")
	}
}

func TestShareWholePercent(t *testing.T) {
	for _, tt := range []struct {
		s    Share
		pct  int
		whol bool
	}{{Share{1, 2}, 50, true}, {Share{33, 50}, 66, true}, {Share{1, 3}, 0, false}, {Share{1, 1}, 100, true}, {Share{}, 0, false}} {
		if p, ok := tt.s.WholePercent(); p != tt.pct || ok != tt.whol {
			t.Errorf("%v WholePercent = %d, %v", tt.s, p, ok)
		}
	}
}

// limitReport decodes the limits_followers fixture with its bow limit's share keys replaced by keys (a JSON fragment).
func limitReport(t *testing.T, keys string) (*Document, Report) {
	t.Helper()
	raw, err := os.ReadFile("testdata/valid/limits_followers.json")
	if err != nil {
		t.Fatal(err)
	}
	const was = `"share": "1/3",`
	if !strings.Contains(string(raw), was) {
		t.Fatalf("the fixture has no %s", was)
	}
	doc, _, rep := Decode(fixtureFile, []byte(strings.Replace(string(raw), was, keys, 1)), newFakeLower())
	return doc, rep
}

func TestValidateLimitShares(t *testing.T) {
	cases := []struct {
		name string
		keys string
		want string // an issue substring, "" for a clean report
		sev  Severity
	}{
		{"fraction only", `"share": "1/3",`, "", Error},
		{"percent only", `"maximumPercentage": 50,`, "", Error},
		{"percent near a third warns", `"maximumPercentage": 33.33,`, "33.33 looks like 1/3: write `1/3`", Warn},
		{"both", `"share": "1/3", "maximumPercentage": 33.33,`, "not both", Error},
		{"neither", ``, "needs a share", Error},
		{"zero share", `"share": "0/3",`, "above 0", Error},
		{"denominator zero", `"share": "1/0",`, "denominator", Error},
		{"more than the whole", `"share": "4/3",`, "more than the whole", Error},
		{"not a fraction", `"share": "a third",`, "not a fraction", Error},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, rep := limitReport(t, tt.keys)
			if tt.want == "" {
				if len(rep.Issues) != 0 {
					t.Fatalf("want a clean report, got:\n%s", rep.String())
				}
				return
			}
			for _, i := range rep.Issues {
				if strings.Contains(i.Message, tt.want) && i.Severity == tt.sev {
					return
				}
			}
			t.Errorf("want %v %q, got:\n%s", tt.sev, tt.want, rep.String())
		})
	}
}

func TestLimitSharesGiveIdenticalLimits(t *testing.T) {
	for _, keys := range []string{`"share": "1/3",`, `"maximumPercentage": 33.33,`} {
		doc, rep := limitReport(t, keys)
		if !rep.OK() {
			t.Fatal(rep.String())
		}
		s, err := doc.Limits[0].LimitShare()
		if err != nil {
			t.Fatal(err)
		}
		for n := 1; n <= 100; n++ {
			if got, want := s.Of(n, true), (n+2)/3; got != want {
				t.Errorf("%s: %d models allow %d, want %d", keys, n, got, want)
			}
		}
	}
}

// typedReport decodes the company_typed fixture with one text replaced.
func typedReport(t *testing.T, was, now string) (*Document, Report) {
	t.Helper()
	raw, err := os.ReadFile("testdata/valid/company_typed.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), was) {
		t.Fatalf("the fixture has no %s", was)
	}
	doc, _, rep := Decode(fixtureFile, []byte(strings.Replace(string(raw), was, now, 1)), newFakeLower())
	return doc, rep
}

const defaultBreak = `"percent": 50,
        "of": "startingModels"`

func TestCompanyShares(t *testing.T) {
	cases := []struct {
		name, was, now, want string
		sev                  Severity
	}{
		{"override fraction", `"bow": 50`, `"bow": "1/2"`, "", Error},
		{"override percent near a third warns", `"bow": 50`, `"bow": 66.67`, "66.67 looks like 2/3: write `2/3`", Warn},
		{"override bad fraction", `"bow": 50`, `"bow": "1/0"`, "denominator", Error},
		{"keyword fraction", `"percent": 33,`, `"share": "33/100",`, "", Error},
		{"keyword both", `"percent": 33,`, `"percent": 33, "share": "1/3",`, "not both", Error},
		{"keyword neither", `"percent": 33,`, ``, "needs a share", Error},
		{"keyword near a third", `"percent": 33,`, `"percent": 33.33,`, "33.33 looks like 1/3", Warn},
		{"default break point fraction", defaultBreak, `"share": "1/2", "of": "startingModels"`, "", Error},
		{"default break point not a whole percent", defaultBreak, `"share": "1/3", "of": "startingModels"`, "whole percent", Error},
		{"default break point both", defaultBreak, `"percent": 50, "share": "1/2", "of": "startingModels"`, "not both", Error},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, rep := typedReport(t, tt.was, tt.now)
			if tt.want == "" {
				if len(rep.Issues) != 0 {
					t.Fatalf("want a clean report, got:\n%s", rep.String())
				}
				return
			}
			for _, i := range rep.Issues {
				if strings.Contains(i.Message, tt.want) && i.Severity == tt.sev {
					return
				}
			}
			t.Errorf("want %v %q, got:\n%s", tt.sev, tt.want, rep.String())
		})
	}
}

func TestLegacyPercentsConvertExactly(t *testing.T) {
	doc, rep := typedReport(t, `"bow": 50`, `"bow": 50`)
	if !rep.OK() {
		t.Fatal(rep.String())
	}
	var c *Company
	for i := range doc.Companies {
		if doc.Companies[i].Limits != nil && len(doc.Companies[i].Limits.Override) > 0 {
			c = &doc.Companies[i]
		}
	}
	if c == nil {
		t.Fatal("no company with an override")
	}
	if got := c.Limits.Override["bow"]; got != (Share{1, 2}) {
		t.Errorf("override bow 50 = %v, want 1/2", got)
	}
	if s, _ := c.Limits.Company[0].KeywordShare(); s != (Share{33, 100}) {
		t.Errorf("keyword 33 = %v, want 33/100", s)
	}
	if s, _ := doc.Campaign.BreakPoint.Default.DefaultShare(); s != (Share{1, 2}) {
		t.Errorf("default break point 50 = %v, want 1/2", s)
	}
}
