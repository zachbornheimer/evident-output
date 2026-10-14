#!/usr/bin/env python3
"""Cross-check real evo output against the published schemas with a stock validator.

Non-gating: the Go test TestPublishedSchemasValidateRealOutputStrictly is the gate. This
script asks the same question with a validator this repo does not own, which is what an
ajv or jsonschema consumer would run.

    mkdir -p "$DIR" && EVO_REAL_OUTPUT_DIR="$DIR" go test . -run TestPublishedSchemasValidateRealOutputStrictly -count=1
    uv run --with jsonschema python -I tools/crosscheck-schemas.py "$DIR"
"""

import json
import sys
from pathlib import Path

import jsonschema

SCHEMA_DIR = Path(__file__).resolve().parent.parent / "schema"


def validator(name: str) -> jsonschema.Draft202012Validator:
    schema = json.loads((SCHEMA_DIR / name).read_text())
    jsonschema.Draft202012Validator.check_schema(schema)
    return jsonschema.Draft202012Validator(schema)


def violations(check: jsonschema.Draft202012Validator, document: object) -> list[str]:
    return [
        f"{'/'.join(map(str, e.absolute_path)) or '<root>'}: {e.message}"
        for e in check.iter_errors(document)
    ]


def main(captured: Path) -> int:
    run, event = validator("run.v3.json"), validator("event.v2.json")
    documents = events = failures = 0
    for path in sorted(captured.glob("*.json")):
        documents += 1
        for message in violations(run, json.loads(path.read_text())):
            failures += 1
            print(f"{path.name}: {message}")
    for path in sorted(captured.glob("*.jsonl")):
        for number, line in enumerate(path.read_text().splitlines(), 1):
            events += 1
            for message in violations(event, json.loads(line)):
                failures += 1
                print(f"{path.name}:{number}: {message}")
    print(
        f"checked {documents} evo.run documents and {events} evo.event lines: {failures} violation(s)"
    )
    return 1 if failures or not (documents and events) else 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    sys.exit(main(Path(sys.argv[1])))
