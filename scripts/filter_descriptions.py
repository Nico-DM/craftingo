#!/usr/bin/env python3
"""Write data/descriptions.md with only the descriptions the catalog uses.

The full Markdown dump is not committed. Keep it in data/descriptions.full.md
(or pass another path) and run this script after changing data/items.tsv:

    python3 scripts/filter_descriptions.py [full-dump.md]

It drops the properties line and the trailing `***Base items.***` section,
picks the right variant for duplicate headings, and prints the items that have
no description. Entries already present in data/descriptions.md are kept for
items the full dump does not cover and for ambiguous duplicate headings, so
one-off additions and prior choices survive a rerun.
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import build_dataset as bd


def main() -> int:
    source = (
        Path(sys.argv[1])
        if len(sys.argv) > 1
        else bd.DATA / "descriptions.full.md"
    )
    if not source.exists():
        print(f"missing full dump: {source}", file=sys.stderr)
        return 1

    descriptions = bd.parse_descriptions(source.read_text())
    lookup = bd.description_lookup(descriptions)

    items = bd.load_items()
    bd.add_synthetic_tier_templates(items)

    out = bd.DATA / "descriptions.md"
    existing = bd.parse_descriptions(out.read_text()) if out.exists() else {}
    existing_lookup = bd.description_lookup(existing)

    used: dict[str, str] = {}
    missing: list[dict] = []
    for item in items:
        heading = bd.description_heading(item, descriptions, lookup)
        if heading is None:
            # Keep a manual entry added directly to the reduced file.
            heading = bd.description_heading(item, existing, existing_lookup)
            if heading is None:
                missing.append(item)
                continue
            used[heading] = bd.pick_description(existing[heading])
            continue
        used[heading] = bd.pick_description(
            descriptions[heading], existing.get(heading)
        )

    out.write_text(
        "".join(
            f"#### {heading}\n\n---\n\n{used[heading]}\n\n"
            for heading in sorted(used)
        )
    )

    print(f"wrote {out} ({len(used)} descriptions)")
    if missing:
        print(f"missing descriptions ({len(missing)}):")
        for item in missing:
            print(f"  - {item['name']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
