#!/usr/bin/env python3
"""Pair data/master.tsv items with data/scraped.tsv entries.

Outputs:
  data/items.tsv         one row per master item, with matched scraped data
  data/match_review.tsv  unmatched / ambiguous / variant-base matches to review

No third-party dependencies.
"""

from __future__ import annotations

import argparse
import csv
import difflib
import re
import sys
import unicodedata
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

SUBTYPE_WORDS = {
    "sword", "axe", "mace", "hammer", "spear", "dagger", "club", "staff",
    "wand", "rod", "slashing", "piercing", "bludgeoning", "melee", "ranged",
    "simple", "martial", "light", "heavy", "two", "handed", "versatile",
    "finesse", "thrown", "ammunition", "firearm", "shield", "armor", "weapon",
}

SOURCE_NAMES = {
    "DMG'14": "Dungeon Master's Guide",
    "PHB'14": "Player's Handbook",
    "MM'14": "Monster Manual",
    "TCE": "Tasha's Cauldron of Everything",
    "XGE": "Xanathar's Guide to Everything",
    "FTD": "Fizban's Treasury of Dragons",
    "EGW": "Explorer's Guide to Wildemount",
    "ERLW": "Eberron: Rising from the Last War",
    "CRCotN": "Critical Role: Call of the Netherdeep",
    "AI": "Acquisitions Incorporated",
    "BGDIA": "Baldur's Gate: Descent into Avernus",
    "TDCSR": "Tal'Dorei Campaign Setting Reborn",
    "BGG": "Bigby Presents: Glory of the Giants",
    "VGM": "Volo's Guide to Monsters",
    "GGR": "Guildmasters' Guide to Ravnica",
    "HWCS": "Humblewood Campaign Setting",
    "MOT": "Mythic Odysseys of Theros",
}


def canonical(text: str) -> str:
    text = (text or "").replace("\u2019", "'").replace("\u2018", "'").replace("`", "'")
    text = unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode("ascii")
    text = text.lower().replace("&", " and ")
    text = text.replace("armour", "armor").replace("pyschic", "psychic")
    text = text.replace("'", "")
    text = re.sub(r"[^a-z0-9+]+", " ", text)
    return re.sub(r"\s+", " ", text).strip()


def is_quantity_paren(content: str) -> bool:
    c = content.strip().lower()
    if not c:
        return False
    if re.fullmatch(r"\d[\d,]*", c):
        return True
    if re.fullmatch(r"\d[\d,]*\s*(?:-|\s)?\s*(?:days?|sheets?|feet|foot|ft\.?|pinch(?:es)?|doses?|oz\.?|lb\.?|pounds?)", c):
        return True
    if re.fullmatch(r"(?:set|bag) of \d[\d,]*", c):
        return True
    if c in {"pair", "full", "empty", "one sheet", "per day"}:
        return True
    return bool(re.fullmatch(r"\d[\d,]*\s*,\s*[a-z][a-z /-]*", c))


def is_subtype_paren(content: str) -> bool:
    words = re.findall(r"[a-z]+", content.lower())
    return bool(words) and all(w in SUBTYPE_WORDS for w in words)


def strip_quantity_parens(text: str) -> str:
    return re.sub(
        r"\s*\(([^)]*)\)",
        lambda m: "" if is_quantity_paren(m.group(1)) else m.group(0),
        text,
    )


def strip_subtype_parens(text: str) -> str:
    return re.sub(
        r"\s*\(([^)]*)\)",
        lambda m: "" if is_subtype_paren(m.group(1)) else m.group(0),
        text,
    )


def strip_all_parens(text: str) -> str:
    return re.sub(r"\s*\([^)]*\)", "", text)


def split_top_level_comma(text: str):
    depth = 0
    for i, ch in enumerate(text):
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth = max(0, depth - 1)
        elif ch == "," and depth == 0:
            return text[:i], text[i + 1:]
    return None, None


def comma_swap(text: str) -> str | None:
    before, after = split_top_level_comma(text)
    if before is None or after is None:
        return None
    return f"{after.strip()} {before.strip()}"


def comma_base(text: str) -> str | None:
    before, _ = split_top_level_comma(text)
    return before.strip() if before is not None else None


def plus_moves(key: str) -> list[str]:
    tokens = key.split()
    plus = [t for t in tokens if re.fullmatch(r"\+\d+", t)]
    if not plus:
        return []
    rest = [t for t in tokens if not re.fullmatch(r"\+\d+", t)]
    return [" ".join(plus + rest), " ".join(rest + plus)]


def singular_variants(key: str) -> list[str]:
    out = []
    tokens = key.split()
    for i, token in enumerate(tokens):
        if len(token) >= 4 and token.endswith("s") and not token.endswith("ss"):
            out.append(" ".join(tokens[:i] + [token[:-1]] + tokens[i + 1:]))
    return out


def trailing_generic_variants(key: str) -> list[str]:
    out = []
    tokens = key.split()
    if len(tokens) > 1 and tokens[-1] in {"armor", "weapon", "shield", "ammunition"}:
        out.append(" ".join(tokens[:-1]))
    return out


def slugify(name: str) -> str:
    text = strip_subtype_parens(strip_quantity_parens(name))
    text = text.replace("+", " plus ").replace("&", " and ")
    text = unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode("ascii")
    text = text.lower().replace("'", "")
    text = re.sub(r"[^a-z0-9]+", "-", text)
    return re.sub(r"-+", "-", text).strip("-")


def load_tsv(path: Path) -> list[dict]:
    with path.open(newline="", encoding="utf-8") as f:
        return list(csv.DictReader(f, delimiter="\t"))


def build_indexes(scraped: list[dict]):
    full = defaultdict(list)
    stripped = defaultdict(list)
    for row in scraped:
        full[canonical(row["Name"])].append(row)
        stripped[canonical(strip_all_parens(row["Name"]))].append(row)
    return full, stripped


def prefer_clean(rows):
    """Drop '(*)' marker duplicates when a normal entry exists."""
    clean = [row for row in rows if "(*)" not in row["Name"]]
    return clean or rows


def lookup(key: str, full, stripped):
    """Return (rows, detail_suffix). Prefer exact canonical matches."""
    rows = full.get(key)
    if rows:
        return prefer_clean(rows), ""
    rows = stripped.get(key)
    if rows:
        return prefer_clean(rows), "+scraped_paren_strip"
    return [], ""


def quantity_plus_subtype(raw: str) -> str | None:
    """Turn 'Item (9, Sword)' into 'Item Sword'."""
    before, _ = split_top_level_comma(raw)
    if before is not None:
        return None
    match = re.search(r"\(([^)]*)\)", raw)
    if not match:
        return None
    content = match.group(1).strip()
    m = re.fullmatch(r"\d[\d,]*\s*,\s*(.+)", content)
    if not m or not is_subtype_paren(m.group(1)):
        return None
    return f"{strip_all_parens(raw)} {m.group(1)}"


def candidates_for(raw: str):
    """Return {(stage, key): (cost, detail)}.

    stage 2 = strong normalization, 3 = weak normalization,
    4 = base/variant fallback.
    """
    best: dict[tuple[int, str], tuple[int, str]] = {}

    def add(stage: int, cost: int, detail: str, text: str | None) -> None:
        if not text:
            return
        key = canonical(text)
        if not key:
            return
        old = best.get((stage, key))
        if old is None or cost < old[0]:
            best[(stage, key)] = (cost, detail)

    add(2, 0, "canonical", raw)
    add(2, 0, "quantity+subtype", quantity_plus_subtype(raw))
    add(2, 1, "quantity_strip", strip_quantity_parens(raw))
    add(2, 1, "subtype_strip", strip_subtype_parens(raw))
    add(2, 2, "quantity+subtype_strip", strip_subtype_parens(strip_quantity_parens(raw)))
    swap = comma_swap(raw)
    if swap:
        add(2, 1, "comma_swap", swap)
    swapped_clean = comma_swap(strip_subtype_parens(strip_quantity_parens(raw)))
    if swapped_clean and swapped_clean != swap:
        add(2, 2, "comma_swap+cleanup", swapped_clean)

    for (stage, key), (cost, _detail) in list(best.items()):
        if stage != 2:
            continue
        for moved in plus_moves(key):
            add(2, cost + 1, "plus_move", moved)

    for (stage, key), (cost, _detail) in list(best.items()):
        if stage != 2:
            continue
        for singular in singular_variants(key):
            add(3, cost + 1, "singular", singular)
        for generic in trailing_generic_variants(key):
            add(3, cost + 1, "trailing_generic", generic)

    base = comma_base(raw)
    if base:
        add(4, 1, "comma_base", base)
    if "(" in raw:
        add(4, 1, "paren_base", strip_all_parens(raw))
    base_clean = comma_base(strip_subtype_parens(strip_quantity_parens(raw)))
    if base_clean and base_clean != base:
        add(4, 2, "comma_base+cleanup", base_clean)

    for (stage, key), (cost, _detail) in list(best.items()):
        if stage != 4:
            continue
        for moved in plus_moves(key):
            add(4, cost + 1, "base+plus_move", moved)
        for singular in singular_variants(key):
            add(4, cost + 1, "base+singular", singular)
        for generic in trailing_generic_variants(key):
            add(4, cost + 1, "base+trailing_generic", generic)

    return best


def match_item(raw: str, full, stripped, aliases):
    # 1. curated alias
    alias_target = aliases.get(canonical(raw))
    if alias_target:
        rows, _ = lookup(canonical(alias_target), full, stripped)
        if rows:
            return rows, "alias", "curated_alias", 0.90

    best = candidates_for(raw)
    for stage, method, confidence in (
        (2, "normalized", 0.95),
        (3, "normalized", 0.92),
        (4, "variant_base", 0.80),
    ):
        entries = [
            (cost, detail, key)
            for (s, key), (cost, detail) in best.items()
            if s == stage
        ]
        for cost in sorted({c for c, _, _ in entries}):
            found: dict[str, dict] = {}
            details = []
            for c, detail, key in entries:
                if c != cost:
                    continue
                rows, suffix = lookup(key, full, stripped)
                for row in rows:
                    found[row["Name"]] = row
                if rows:
                    details.append(detail + suffix)
            if not found:
                continue
            if len(found) == 1:
                return [next(iter(found.values()))], method, ";".join(details), confidence
            return list(found.values()), "ambiguous", ";".join(details), 0.0
    return [], "none", "", 0.0


def make_slug(name: str, used: set[str]) -> str:
    base = slugify(name) or "item"
    slug = base
    n = 2
    while slug in used:
        slug = f"{base}-{n}"
        n += 1
    used.add(slug)
    return slug


def write_tsv(path: Path, fieldnames: list[str], rows: list[dict]) -> None:
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--master", type=Path, default=ROOT / "data/master.tsv")
    parser.add_argument("--scraped", type=Path, default=ROOT / "data/scraped.tsv")
    parser.add_argument("--aliases", type=Path, default=ROOT / "data/item_aliases.tsv")
    parser.add_argument("--out", type=Path, default=ROOT / "data/items.tsv")
    parser.add_argument("--review", type=Path, default=ROOT / "data/match_review.tsv")
    args = parser.parse_args()

    master = load_tsv(args.master)
    scraped = load_tsv(args.scraped)
    full, stripped = build_indexes(scraped)
    scraped_by_name = {row["Name"]: row for row in scraped}

    aliases = {}
    alias_notes = {}
    if args.aliases.exists():
        for row in load_tsv(args.aliases):
            key = canonical(row["master_name"])
            target = row["scraped_name"].strip()
            if key in aliases:
                print(f"warning: duplicate alias for {row['master_name']!r}", file=sys.stderr)
            if target not in scraped_by_name:
                print(f"warning: alias target not found in scraped: {target!r}", file=sys.stderr)
                continue
            aliases[key] = target
            alias_notes[key] = (row.get("note") or "").strip()

    out_rows = []
    review_rows = []
    used_aliases = set()
    used_slugs: set[str] = set()
    counts = defaultdict(int)

    for master_row in master:
        name = master_row["name"].strip()
        slug = make_slug(name, used_slugs)
        matched, method, detail, confidence = [], "none", "", 0.0

        if name in scraped_by_name:
            matched, method, detail, confidence = [scraped_by_name[name]], "exact", "exact_name", 1.0
        else:
            matched, method, detail, confidence = match_item(name, full, stripped, aliases)
            if method == "alias":
                used_aliases.add(canonical(name))

        counts[method] += 1
        match_status = {
            "exact": "matched", "alias": "matched", "normalized": "matched",
            "variant_base": "matched", "ambiguous": "ambiguous", "none": "unmatched",
        }[method]

        out = {
            "id": slug,
            "name": master_row["name"],
            "rarity": master_row["rarity"],
            "source": master_row["source"],
            "magic": master_row["magic"],
            "oota": master_row["oota"],
            "type": master_row["type"],
            "recipes": master_row["recipes"],
            "short_description": master_row["short_description"],
            "scraped_name": "",
            "match_status": match_status,
            "match_method": method,
            "match_detail": detail,
            "match_confidence": f"{confidence:.2f}",
            "scraped_source_code": "",
            "scraped_source": "",
            "scraped_rarity": "",
            "scraped_type": "",
            "attunement": "",
            "damage": "",
            "properties": "",
            "weight": "",
            "value": "",
            "scraped_text": "",
        }
        if len(matched) == 1:
            s = matched[0]
            out.update({
                "scraped_name": s["Name"],
                "scraped_source_code": s["Source"],
                "scraped_source": SOURCE_NAMES.get(s["Source"], s["Source"]),
                "scraped_rarity": s["Rarity"],
                "scraped_type": s["Type"],
                "attunement": s["Attunement"],
                "damage": s["Damage"],
                "properties": s["Properties"],
                "weight": s["Weight"],
                "value": s["Value"],
                "scraped_text": s["Text"],
            })
        else:
            reason = "ambiguous" if method == "ambiguous" else "unmatched"
            candidate_names = [row["Name"] for row in matched]
            suggestions = difflib.get_close_matches(name, list(scraped_by_name), n=3, cutoff=0.4)
            suggestions = [x for x in suggestions if x not in candidate_names][:3]
            review_rows.append({
                "id": slug,
                "name": master_row["name"],
                "type": master_row["type"],
                "source": master_row["source"],
                "reason": reason,
                "match_method": method,
                "match_detail": detail,
                "candidates": "; ".join(candidate_names),
                "suggested_1": suggestions[0] if len(suggestions) > 0 else "",
                "suggested_2": suggestions[1] if len(suggestions) > 1 else "",
                "suggested_3": suggestions[2] if len(suggestions) > 2 else "",
            })
        if method == "variant_base" and len(matched) == 1:
            review_rows.append({
                "id": slug,
                "name": master_row["name"],
                "type": master_row["type"],
                "source": master_row["source"],
                "reason": "variant_base_check",
                "match_method": method,
                "match_detail": detail,
                "candidates": matched[0]["Name"],
                "suggested_1": matched[0]["Name"],
                "suggested_2": "",
                "suggested_3": "",
            })
        out_rows.append(out)

    out_fields = [
        "id", "name", "rarity", "source", "magic", "oota", "type", "recipes",
        "short_description", "scraped_name", "match_status", "match_method",
        "match_detail", "match_confidence", "scraped_source_code", "scraped_source",
        "scraped_rarity", "scraped_type", "attunement", "damage", "properties",
        "weight", "value", "scraped_text",
    ]
    review_fields = [
        "id", "name", "type", "source", "reason", "match_method", "match_detail",
        "candidates", "suggested_1", "suggested_2", "suggested_3",
    ]
    write_tsv(args.out, out_fields, out_rows)
    write_tsv(args.review, review_fields, review_rows)

    unused = [key for key in aliases if key not in used_aliases]
    print(f"Loaded {len(master)} master items and {len(scraped)} scraped items")
    for key in ("exact", "alias", "normalized", "variant_base", "ambiguous", "none"):
        if counts[key]:
            print(f"  {key:12s} {counts[key]:4d}")
    print(f"Wrote {args.out} ({len(out_rows)} rows)")
    print(f"Wrote {args.review} ({len(review_rows)} review rows)")
    if unused:
        print(f"note: {len(unused)} aliases not used (already matched or no longer needed)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
