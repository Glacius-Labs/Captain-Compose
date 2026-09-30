"""Check published JSON Schema against the same fixtures used by the Go decoder."""
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker

root = Path(__file__).resolve().parents[2]
schema = json.loads((root / 'schemas/command-v2.schema.json').read_text(encoding='utf-8'))
Draft202012Validator.check_schema(schema)
validator = Draft202012Validator(schema, format_checker=FormatChecker())
cases = json.loads((root / 'tests/contracts/wire-fixtures.json').read_text(encoding='utf-8'))
failures = []
for case in cases:
    errors = list(validator.iter_errors(case['command']))
    if (not errors) != case['valid']:
        failures.append(f"{case['name']}: expected valid={case['valid']}; {errors[0].message if errors else 'accepted invalid request'}")
if failures:
    raise SystemExit('\n'.join(failures))
print(f"JSON Schema conforms to {len(cases)} shared wire fixtures.")
