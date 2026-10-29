package core

// Text the lean format does not store, derived from the rule that says it. The old
// parsers use these helpers too, so a progression row or a power written without its text reads exactly as one that spells it out.

// ruleText is what a Battle Companies special rule says: its tag (Active, Passive or empty) and its definition.
type ruleText struct{ Tag, Definition string }

// ruleTexts are the rules of data.keywords by name, the first definition of a name wins.
type ruleTexts map[string]ruleText

func (t ruleTexts) add(name, tag, definition string) {
	if _, seen := t[name]; !seen {
		t[name] = ruleText{tag, definition}
	}
}

// RuleRowText is the text a progression row shows for the special rule it grants: "Spy (Active) - text", or "Lead by Example - text"
// when the rule has no tag.
func RuleRowText(name, tag, definition string) string {
	if tag != "" {
		return name + " (" + tag + ") - " + definition
	}
	return name + " - " + definition
}

// rowText is the text of a progression row: the one the file writes, else, for a row that grants exactly one special rule (rule
// is its name, "" for any other row) the text derived from the rule. A rule the table does not know leaves the text empty.
func (t ruleTexts) rowText(text, rule string) string {
	if text != "" || rule == "" {
		return text
	}
	if k, ok := t[rule]; ok {
		return RuleRowText(rule, k.Tag, k.Definition)
	}
	return ""
}
