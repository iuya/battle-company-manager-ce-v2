"""The committed docs/customizing-rulesets-reference.md is what tools/gen_ruleset_reference.py makes from the schema."""
import json
import unittest

import gen_ruleset_reference as gen


class RulesetReference(unittest.TestCase):
    def test_committed_reference_is_current(self):
        schema = json.loads(gen.SCHEMA.read_text(encoding="utf-8"))
        have = gen.OUT.read_text(encoding="utf-8")
        self.assertEqual(have, gen.render(schema), "docs/customizing-rulesets-reference.md is stale: run `make ruleset-reference`")

    def test_every_key_of_the_schema_is_listed(self):
        schema = json.loads(gen.SCHEMA.read_text(encoding="utf-8"))
        text = gen.render(schema)
        for name, node in schema["$defs"].items():
            for key in node.get("properties", {}):
                self.assertIn(f"| `{key}` |", text, f"{name}.{key} is missing from the reference")

    def test_output_is_deterministic(self):
        schema = json.loads(gen.SCHEMA.read_text(encoding="utf-8"))
        self.assertEqual(gen.render(schema), gen.render(json.loads(json.dumps(schema))))


if __name__ == "__main__":
    unittest.main()
