package core

// Text the lean format does not store, derived from the rule that says it, so a progression row written without its text reads exactly
// as one that spells it out.

// RuleRowText is the text a progression row shows for the special rule it grants: "Spy (Active) - text", or "Lead by Example - text"
// when the rule has no tag.
func RuleRowText(name, tag, definition string) string {
	if tag != "" {
		return name + " (" + tag + ") - " + definition
	}
	return name + " - " + definition
}
