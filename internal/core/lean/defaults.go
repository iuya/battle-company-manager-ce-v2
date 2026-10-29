package lean

// Normalise applies the extends engine, then the defaults of the format, once. It is idempotent: Normalise on a normalised
// document changes nothing (the property test pins it). Errors of the extends engine (loops, a unit with no name) go to rep.
func Normalise(doc *Document, rep *Reporter) {
	applyExtends(doc, rep)
	applyDefaults(doc)
}

const (
	defaultOwnPath      = "On Their Own Path"
	defaultPathKind     = "heroic"
	defaultScenarioSide = "both"
	defaultTargetApply  = "own"
	defaultTargetMatch  = "any"
)

func applyDefaults(doc *Document) {
	for i := range doc.Rules {
		ruleDefaults(&doc.Rules[i])
	}
	for i := range doc.Gear {
		gearDefaults(&doc.Gear[i])
	}
	if up := doc.ArmourUp; up != nil && up.Gold == nil {
		r := up.Rating
		up.Gold = &r
	}
	for i := range doc.Units {
		unitDefaults(&doc.Units[i])
	}
	for i := range doc.Paths {
		p := &doc.Paths[i]
		p.ID = idOr(p.ID, p.Name)
		if p.Kind == "" {
			p.Kind = defaultPathKind
		}
		if p.Dice == (DiceSpec{}) {
			p.Dice = doc.Campaign.Progression.Hero.Roll
		}
		for _, r := range pathRules(p) {
			ruleDefaults(r)
		}
	}
	for i := range doc.Scenarios {
		s := &doc.Scenarios[i]
		s.ID = idOr(s.ID, s.Name)
		if s.Side == "" {
			s.Side = defaultScenarioSide
		}
		for j := range s.Scoring {
			if s.Scoring[j].Side == "" {
				s.Scoring[j].Side = defaultScenarioSide
			}
		}
	}
	for i := range doc.Companies {
		c := &doc.Companies[i]
		c.ID = idOr(c.ID, c.Name)
		if c.Faction == "" {
			c.Faction = c.Name
		}
		if c.MaxModels == 0 {
			c.MaxModels = doc.Campaign.Company.MaxModels
		}
		if c.OwnPath != nil && c.OwnPath.Path == "" {
			c.OwnPath.Path = defaultOwnPath
		}
		if c.Start != nil {
			for j := range c.Start.Units {
				if c.Start.Units[j].Count == 0 {
					c.Start.Units[j].Count = 1
				}
			}
		}
		if c.Reinforcements != nil {
			rowDefaults(c.Reinforcements.Rows)
		}
		if c.Special != nil {
			rowDefaults(c.Special.Rows)
		}
		for j := range c.Units {
			unitDefaults(&c.Units[j])
		}
		for j := range c.Gear {
			gearDefaults(&c.Gear[j])
		}
		for j := range c.Rules {
			ruleDefaults(&c.Rules[j])
		}
	}
}

func idOr(id, name string) string {
	if id != "" {
		return id
	}
	return Slug(name)
}

func rowDefaults(rows []ChartRow) {
	for i := range rows {
		if rows[i].Count == 0 {
			rows[i].Count = 1
		}
	}
}

func ruleDefaults(r *Rule) {
	r.ID = idOr(r.ID, r.Name)
	if r.Target != nil {
		if r.Target.AppliesTo == "" {
			r.Target.AppliesTo = defaultTargetApply
		}
		if r.Target.Match == "" {
			r.Target.Match = defaultTargetMatch
		}
	}
}

func gearDefaults(g *Gear) {
	g.ID = idOr(g.ID, g.Name)
	if g.Gold == nil {
		r := g.Rating
		g.Gold = &r
	}
}

func unitDefaults(u *Unit) {
	u.ID = idOr(u.ID, u.Name)
	if u.OwnPath != nil && u.OwnPath.Path == "" {
		u.OwnPath.Path = defaultOwnPath
	}
}

// pathRules lists the inline rule definitions of a path's rows (hoisted by the resolution pass).
func pathRules(p *Path) []*Rule {
	var out []*Rule
	var walk func(opts []PathOption)
	walk = func(opts []PathOption) {
		for i := range opts {
			for _, ref := range []*RuleRef{opts[i].Rule, opts[i].MinorRule} {
				if ref != nil && ref.Def != nil {
					out = append(out, ref.Def)
				}
			}
			walk(opts[i].OneOf)
		}
	}
	for i := range p.Rows {
		walk(p.Rows[i].Options)
	}
	return out
}
