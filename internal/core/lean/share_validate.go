package lean

import (
	"fmt"
	"strconv"
	"strings"
)

// LimitShare is the most a limit allows, as a share of the company: its share, else its maximumPercentage converted.
func (l Limit) LimitShare() (Share, error) {
	if l.Share.IsSet() {
		return l.Share, nil
	}
	return PercentFloatShare(l.MaximumPercentage)
}

// KeywordShare is the share of a company that may have the keyword: its share, else its percent converted.
func (k KeywordLimit) KeywordShare() (Share, error) {
	if k.Share.IsSet() {
		return k.Share, nil
	}
	return PercentFloatShare(k.Percent)
}

// DefaultShare is the file's default Break Point: its share, else its percent converted.
func (b BreakDefault) DefaultShare() (Share, error) {
	if b.Share.IsSet() {
		return b.Share, nil
	}
	return PercentFloatShare(float64(b.Percent))
}

// shares checks every share of the document: that a limit has exactly one of its two keys, that no share is 0 where a limit must
// allow something, that a Break Point is a whole percent (the live match counts casualties against whole percents), and warns when a
// percent is the rounding of a simple fraction.
func (v *validator) shares() {
	d := v.doc
	for i, l := range d.Limits {
		p := Idx("limits", i)
		switch {
		case l.Share.IsSet() && l.MaximumPercentage != 0:
			v.rep.Errorf(Join(p, "share"), `write the share of the limit as "share" or as "maximumPercentage", not both`)
		case !l.Share.IsSet() && l.MaximumPercentage == 0:
			v.rep.Add(Issue{Path: p, Message: "the limit needs a share", Hint: `write "share": "1/3" (a fraction of the company)`})
		default:
			s, err := l.LimitShare()
			v.checkShare(s, err, Join(p, shareKey(l.Share, "maximumPercentage")), l.Share.IsSet(), true)
		}
	}
	for i := range d.Companies {
		c := &d.Companies[i]
		p := Idx("companies", i)
		if c.BreakPoint.IsSet() {
			v.wholePercentShare(c.BreakPoint, Join(p, "breakPoint"))
		}
		if c.Limits == nil {
			continue
		}
		for _, id := range SortedKeys(c.Limits.Override) {
			v.checkShare(c.Limits.Override[id], nil, Join(Join(Join(p, "limits"), "override"), id), false, false)
		}
		for j, k := range c.Limits.Company {
			kp := Idx(Join(Join(p, "limits"), "company"), j)
			switch {
			case k.Share.IsSet() && k.Percent != 0:
				v.rep.Errorf(Join(kp, "share"), `write the share as "share" or as "percent", not both`)
			case !k.Share.IsSet() && k.Percent == 0:
				v.rep.Add(Issue{Path: kp, Message: "the keyword limit needs a share", Hint: `write "share": "1/3"; "0/1" allows none`})
			default:
				s, err := k.KeywordShare()
				v.checkShare(s, err, Join(kp, shareKey(k.Share, "percent")), k.Share.IsSet(), false)
			}
		}
	}
	bd := d.Campaign.BreakPoint.Default
	bp := "campaign.breakPoint.default"
	switch {
	case bd.Share.IsSet() && bd.Percent != 0:
		v.rep.Errorf(Join(bp, "share"), `write the Break Point as "share" or as "percent", not both`)
	case !bd.Share.IsSet() && bd.Percent == 0:
		if v.campaignHasErrors() {
			break // the block did not bind; its own errors say why
		}
		v.rep.Add(Issue{Path: bp, Message: "the default Break Point needs a share", Hint: `write "share": "1/2"`})
	default:
		s, _ := bd.DefaultShare()
		v.wholePercentShare(s, Join(bp, shareKey(bd.Share, "percent")))
	}
}

// campaignHasErrors says the report already holds an error inside the campaign block (or its absence).
func (v *validator) campaignHasErrors() bool {
	for _, i := range v.rep.Report.Issues {
		if i.Severity == Error && strings.HasPrefix(i.Path, "campaign") {
			return true
		}
	}
	return false
}

func shareKey(s Share, percentKey string) string {
	if s.IsSet() {
		return "share"
	}
	return percentKey
}

// checkShare reports a failed conversion, a share of 0 where a limit must allow something, and (for a share written as a percent,
// that is not fraction) a percent that nearly is a simple fraction.
func (v *validator) checkShare(s Share, err error, at string, fraction, mustBePositive bool) {
	switch {
	case err != nil:
		v.rep.Errorf(at, "%s", err.Error())
	case mustBePositive && s.Num == 0:
		v.rep.Errorf(at, "the share must be above 0")
	case !fraction:
		if f, ok := s.NearMiss(); ok {
			v.rep.Add(Issue{Path: at, Severity: Warn, Message: fmt.Sprintf("%s looks like %s: write `%s`", percentText(s), f, f),
				Hint: "a percent is read as the exact fraction over 100, which is not a third; write the fraction"})
		}
	}
}

// wholePercentShare checks a Break Point share: above 0, and a whole percent.
func (v *validator) wholePercentShare(s Share, at string) {
	switch _, whole := s.WholePercent(); {
	case s.Num == 0:
		v.rep.Errorf(at, "the Break Point must be above 0")
	case !whole:
		v.rep.Add(Issue{Path: at, Message: fmt.Sprintf("the Break Point %s is %.2f percent; it must be a whole percent", s, s.Percent()), Hint: `write a fraction that is a whole percent: "1/2", "33/50"`})
	}
}

// percentText is the percent of a share the way a person wrote it (33.33).
func percentText(s Share) string { return strconv.FormatFloat(s.Percent(), 'f', -1, 64) }
