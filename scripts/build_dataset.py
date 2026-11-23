#!/usr/bin/env python3
"""Normalize data/items.tsv into the canonical catalog JSON consumed by cmd/seed.

Canonical model
---------------
A recipe is a list of *requirement groups*. Each group asks for `quantity` units
chosen from one of its *options*, and every unit in the group must match the same
option (homogeneous options). An option lists allowed forms, traits and tags. A
unit can satisfy several groups at once, so the calculator only needs per-group
counts.

Everything the source data expresses ambiguously is resolved here: alternatives
are merged into option groups, deprecated forms are replaced, `As X` inheritance
is expanded, and anything that cannot be merged is reported instead of guessed.

Usage:
    python3 scripts/build_dataset.py
Outputs:
    data/build/items.jsonl    canonical items + recipes
    data/build/recipes.txt    one line per item, for eyeballing
    data/build/report.md      merge/conflict report
"""

from __future__ import annotations

import copy
import csv
import json
import re
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DATA = ROOT / "data"
OUT = DATA / "build"

# ---------------------------------------------------------------------------
# Vocabulary
# ---------------------------------------------------------------------------

TRAITS = [
    "Acidic", "Chaotic", "Cold", "Dimensional", "Fiendish", "Fiery",
    "Forceful", "Inert", "Lightning", "Mercurial", "Necrotic", "Phantasmal",
    "Poisonous", "Psychic", "Radiant", "Theurgic", "Thundering", "Vital",
    "Void", "Zephyrous",
]
TRAIT_SET = set(TRAITS)

RAW_FORMS = ["Bone", "Fluid", "Food", "Glass", "Hide", "Metal", "Mineral",
             "Textile", "Wood"]
RAW_SET = set(RAW_FORMS)

# Deprecated form -> current form. Kept in the source data, rewritten on import.
DEPRECATED = {
    "Hardwood": "Wood",
    "Gem": "Mineral",
    "Leather": "Hide",
    "Flesh": "Hide",
    "Foodstuff": "Food",
    "Plant": "Wood",
    "Spice": "Food",
    "Dye": "Fluid",
    "Earthenware": "Glass",
    "Wax": "Fluid",
}
# A deprecated form that needs a tag to stay distinct after mapping.
DEPRECATED_TAGS = {"Wax": ["Bug"]}
# Not one of the nine forms; closest equivalent.
EXTRA_FORMS = {"Feather": "Textile"}
EXTRA_FORM_KEYS = {k.lower(): v for k, v in EXTRA_FORMS.items()}

GENERIC_FORMS = {"Weapon", "Armor", "Ammunition", "Focus", "Instrument"}
MATERIAL_TAGS = RAW_SET | set(DEPRECATED) | set(EXTRA_FORMS)

# Tags are freeform text, but the same concept must not appear twice under
# different spellings. Deprecated form names are rewritten here exactly like
# forms are, and a few casing variants are unified.
TAG_ALIASES = {
    "hardwood": "Wood",
    "gem": "Mineral",
    "leather": "Hide",
    "flesh": "Hide",
    "foodstuff": "Food",
    "plant": "Wood",
    "spice": "Food",
    "dye": "Fluid",
    "earthenware": "Glass",
    "wax": "Fluid",
    "10-foot": "10-foot",
    "10 foot": "10-foot",
    "sword": "Sword",
    "axe": "Axe",
    "dragon": "Dragon",
    "bug": "Bug",
    "slashing": "Slashing",
}
TAG_DISPLAY = {t.lower(): t for t in RAW_FORMS + TRAITS + sorted(GENERIC_FORMS)}


def canon_tag(tag: str) -> str:
    tag = re.sub(r"\s+", " ", tag).strip()
    key = tag.lower()
    if key in TAG_ALIASES:
        return TAG_ALIASES[key]
    return TAG_DISPLAY.get(key, tag)


# Crafted items that other recipes consume as material units. For these, the
# material tags of the crafted item are inherited from the group that describes
# what it is made of (see `apply_output_tags`).
TAG_SOURCE_ITEMS = {
    "Staff", "Shield", "Amulet", "Bottle", "Flask or Tankard", "Vial",
    "Waterskin", "Drum", "Arrows (20)", "Dice Set", "Playing Card Set",
    "Javelin", "Pike", "Staff of Striking", "Staff of Power",
    "Dragon Vessel, Slumbering", "Dragon Vessel, Stirring",
    "Dragon Vessel, Wakened", "Dragon Vessel, Ascendant",
    "Dragon's Wrath Weapon, Slumbering", "Dragon's Wrath Weapon, Stirring",
    "Dragon's Wrath Weapon, Wakened", "Dragon's Wrath Weapon, Ascendant",
    "Dragon-Touched Focus, Slumbering", "Dragon-Touched Focus, Stirring",
    "Dragon-Touched Focus, Wakened", "Dragon-Touched Focus, Ascendant",
}

# Items that are themselves a raw material for another recipe, even though they
# have their own recipe (paper and parchment are processed hide/wood).
MATERIAL_ITEM_IDS = {"paper": "Paper", "parchment": "Parchment"}

# Dragon materials: trait and allowed raw forms, used for validation/reference.
DRAGONS = {
    "Black": ("Acidic", ["Bone", "Hide", "Fluid"]),
    "Blue": ("Lightning", ["Bone", "Hide", "Fluid"]),
    "Green": ("Poisonous", ["Bone", "Hide", "Fluid"]),
    "Red": ("Fiery", ["Bone", "Hide", "Fluid"]),
    "White": ("Cold", ["Bone", "Hide", "Fluid"]),
    "Brass": ("Fiery", ["Bone", "Hide", "Fluid"]),
    "Bronze": ("Lightning", ["Bone", "Hide", "Fluid"]),
    "Copper": ("Acidic", ["Bone", "Hide", "Fluid"]),
    "Gold": ("Fiery", ["Bone", "Hide", "Fluid"]),
    "Silver": ("Cold", ["Bone", "Hide", "Fluid"]),
    "Amethyst": ("Forceful", ["Mineral"]),
    "Crystal": ("Radiant", ["Mineral"]),
    "Emerald": ("Psychic", ["Mineral"]),
    "Sapphire": ("Thundering", ["Mineral"]),
    "Topaz": ("Necrotic", ["Mineral"]),
    "Brass (metallic)": ("Fiery", ["Metal"]),
}

# Recipe names that mean a catalog item but are spelled differently. These are
# source-data fixes, not vocabulary: the long-term home is data/items.tsv.
ITEM_ALIASES = {
    "Arrows": "Arrows (20)",
    "Blowgun Needles": "Blowgun Needles (50)",
    "Crossbow Bolts": "Crossbow Bolts (20)",
    "Sling Bullets": "Sling Bullets (20)",
    "Candle": "Candle (5)",
    "Case": "Case, Map or Scroll",
    "Jug": "Jug or Pitcher",
    "Flask": "Flask or Tankard",
    "Tankard": "Flask or Tankard",
    "Hooded Lantern": "Lantern, Hooded",
    "Hempen Rope": "Rope, Hempen (50 feet)",
    "Silk Rope": "Rope, Silk (50 feet)",
    "Clothes": "Clothes, Common",
    "Traveler's Clothes": "Clothes, Travelers",
    "Prayer Bead": "Prayer Beads, Necklace of",
    "Oil of Slipperiness": "Oil of Slipperiness (3)",
    "Potion of Healing": "Potion of Healing (3)",
    "Potion of Greater Healing": "Potion of Greater Healing (3)",
    "Potion of Superior Healing": "Potion of Superior Healing (3)",
    "Potion of Supreme Healing": "Potion of Supreme Healing (3)",
    "Elixir of Health": "Elixir of Health (3)",
    "Antitoxin": "Antitoxin",
}

WORD_FIXES = {"Posionous": "Poisonous", "Fuid": "Fluid"}

# Weapon bases get the grouping tag the recipes use ("Sword of Wounding" needs a
# "Sword" weapon). Curated; append as needed.
BASE_TAGS = {
    "Longsword": ["Sword"], "Shortsword": ["Sword"], "Greatsword": ["Sword"],
    "Rapier": ["Sword"], "Scimitar": ["Sword"], "Claymore": ["Sword"],
    "Battleaxe": ["Axe"], "Greataxe": ["Axe"], "Handaxe": ["Axe"],
    "Halberd": ["Axe"], "Glaive": ["Axe"],
}

# Explicit recipe rewrites agreed with the data owner.
RECIPE_OVERRIDES = {
    "Flask or Tankard": ["1x Metal, 2x Glass/Hide/Bone/Metal"],
    "Antitoxin": ["3x Fluid, 3x Vital"],
    "Potion of Healing (3)": ["3x Fluid, 3x Vital"],
    "Potion of Greater Healing (3)": ["3x Fluid, 3x Vital"],
    "Potion of Superior Healing (3)": ["3x Fluid, 3x Vital"],
    "Potion of Supreme Healing (3)": ["3x Fluid, 3x Vital"],
    "Elixir of Health (3)": ["3x Fluid, 3x Vital"],
    "Staff of Thunder and Lightning": [
        "1x (Metal/Bone/Wood) Staff, 3x Lightning, 4x Thundering"
    ],
    "Dragon Wing Bow": [
        "1x +2 Shortbow/Longbow, 1x (Dragon) Material, 7x [Trait] (of the same trait)"
    ],
}

QTY_RE = re.compile(r"^(\d+)(?:\s*-\s*(\d+))?\s*x\s+(.+)$", re.I)
GP_RE = re.compile(r"^([\d,]+)\s*(?:x\s*)?GP$", re.I)
RARITY_RE = re.compile(r"^(C|UC|R|VR|L)\s+(.+)$")
TIER_RE = re.compile(r"^\+(\d)\s+(.+)$")
PAREN_RE = re.compile(r"\(([^()]*)\)")
LEADING_PAREN_RE = re.compile(r"^\(([^()]*)\)\s*")
AS_RE = re.compile(r"^As\s+(?:\(([^()]*)\)\s*)?(.+)$")
TRAILING_COUNT_RE = re.compile(r"\s*\(\s*\d[\d,]*[^)]*\)\s*$")
NARRATIVE_RE = re.compile(r"magic circle|Djinni", re.I)

# ---------------------------------------------------------------------------
# Small helpers
# ---------------------------------------------------------------------------


def split_top(text: str, sep: str = ",") -> list[str]:
    """Split on `sep` outside parentheses."""
    out, depth, cur = [], 0, ""
    for ch in text:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth = max(0, depth - 1)
        if ch == sep and depth == 0:
            out.append(cur)
            cur = ""
        else:
            cur += ch
    out.append(cur)
    return [p.strip() for p in out if p.strip()]


def split_alternatives(text: str) -> list[str]:
    """Split on `/` outside parentheses (form/trait alternatives in one token)."""
    return split_top(text, "/")


def clean(text: str) -> str:
    text = re.sub(r"\s+", " ", text).strip()
    text = re.sub(r"^1\+\s+", "+1 ", text)
    for bad, good in WORD_FIXES.items():
        text = re.sub(rf"\b{bad}\b", good, text)
    # "Acid Metal" is a slip for the Acidic trait; bare "Acid" is an item.
    text = re.sub(r"\bAcid\s+(?=[A-Z])", "Acidic ", text)
    return text


def name_keys(name: str) -> list[str]:
    """Loose keys for item lookup: counts stripped, 'A, B' also tried as 'B A'."""
    n = name.strip()
    m = re.match(r"^\+(\d)\s+(.+)$", n)
    if m:
        n = f"{m.group(2)} +{m.group(1)}"
    keys = [n]
    base, sep, suffix = n.partition(", ")
    if sep:
        keys.append(f"{suffix} {base}")
    out = []
    for k in keys:
        k = TRAILING_COUNT_RE.sub("", k)
        out.append(re.sub(r"\s+", " ", k).strip().lower())
    return out


class ItemIndex:
    def __init__(self, items: list[dict]):
        self.by_name = {r["name"]: r for r in items}
        self.by_id = {r["id"]: r for r in items}
        self.by_key: dict[str, dict] = {}
        for r in items:
            for k in name_keys(r["name"]):
                self.by_key.setdefault(k, r)

    def find(self, name: str):
        if name in self.by_name:
            return self.by_name[name]
        alias = ITEM_ALIASES.get(name)
        if alias and alias in self.by_name:
            return self.by_name[alias]
        for k in name_keys(name):
            if k in self.by_key:
                return self.by_key[k]
        return None

    def find_tiered(self, name: str):
        """'+2 Dagger' -> (Dagger item, 2); '+1 Sickle' -> (Sickle item, 1)."""
        m = re.match(r"^\+(\d)\s+(.+)$", name)
        if not m:
            m = re.match(r"^(.+?)\s+\+(\d)$", name)
            if not m:
                return None, None
            base, tier = m.group(1), int(m.group(2))
        else:
            tier, base = int(m.group(1)), m.group(2)
        return self.find(base), tier


def item_intrinsic_tags(name: str, index: ItemIndex | None = None) -> list[str]:
    tags = []
    for m in re.finditer(r"\(([^()]*)\)\s*$", name):
        content = m.group(1).strip()
        for part in re.split(r"[,/]", content):
            part = part.strip()
            if not part or re.fullmatch(r"[\d,]+", part):
                continue
            tag = canon_tag(part)
            if tag not in tags:
                tags.append(tag)
    if index:
        base = re.sub(r"\s*\([^()]*\)\s*$", "", name).strip()
        for t in BASE_TAGS.get(base, []):
            tag = canon_tag(t)
            if tag not in tags:
                tags.append(tag)
    return tags


# ---------------------------------------------------------------------------
# Token parsing
# ---------------------------------------------------------------------------


def new_option() -> dict:
    return {
        "any_raw": False,
        "forms": [],        # [{"kind": raw|item|generic, "name": ..., "id": ...}]
        "traits": [],
        "tags_all": [],
        "tags_any": [],
        "output_tags": [],      # tags the crafted item definitely carries
        "output_tags_any": [],  # tags the crafter picks one of
        "tier": None,
    }


def normalize_option(opt: dict) -> dict:
    """Canonicalise and de-duplicate tags/forms/traits in place."""
    for key in ("tags_all", "tags_any"):
        seen = []
        for t in opt[key]:
            tag = canon_tag(t)
            if tag not in seen:
                seen.append(tag)
        opt[key] = seen
    # A tag that is required cannot also be a free choice.
    opt["tags_any"] = [t for t in opt["tags_any"] if t not in opt["tags_all"]]

    seen_forms, forms = set(), []
    for f in opt["forms"]:
        sig = (f["kind"], f.get("id") or f["name"])
        if sig not in seen_forms:
            seen_forms.add(sig)
            forms.append(f)
    opt["forms"] = forms

    seen_traits, traits = set(), []
    for t in opt["traits"]:
        if t not in seen_traits:
            seen_traits.add(t)
            traits.append(t)
    opt["traits"] = traits
    return opt


def classify_part(part: str, index: ItemIndex):
    """Return (option fields, note) for one alternative of a token."""
    tags_all, tags_any = [], []
    note = None

    # leading parenthesised qualifiers were already peeled off by the caller
    inner = PAREN_RE.findall(part)
    name = PAREN_RE.sub(" ", part)
    name = clean(name)
    for content in inner:
        for chunk in content.split(","):
            chunk = chunk.strip()
            if not chunk:
                continue
            if "/" in chunk:
                for x in chunk.split("/"):
                    x = x.strip()
                    if x and x not in tags_any:
                        tags_any.append(x)
            elif chunk not in tags_all:
                tags_all.append(chunk)

    opt = new_option()
    opt["tags_all"], opt["tags_any"] = tags_all, tags_any

    if not name:
        return opt, "empty"

    # trait variable placeholder
    if name in ("[Trait]", "Trait") or "of the same trait" in name:
        opt["traits"] = []  # filled by the group, not the option
        return opt, "variable"

    # leading trait word ("Poisonous Fluid", "Dimensional Amulet")
    words = name.split()
    if words and words[0] in TRAIT_SET:
        opt["traits"].append(words[0])
        name = " ".join(words[1:]).strip()
        if not name:
            return opt, None

    # tier prefix ("+2 Dagger")
    m = TIER_RE.match(name)
    if m:
        opt["tier"] = int(m.group(1))
        name = m.group(2).strip()

    if not name:
        return opt, "empty"

    # Material = any raw form
    if name == "Material":
        opt["any_raw"] = True
        return opt, None

    if name in GENERIC_FORMS:
        opt["forms"].append({"kind": "generic", "name": name})
        return opt, None

    if name in TRAIT_SET:
        opt["traits"].append(name)
        return opt, None

    if name in RAW_SET:
        opt["forms"].append({"kind": "raw", "name": name})
        return opt, None

    if name in DEPRECATED:
        opt["forms"].append({"kind": "raw", "name": DEPRECATED[name]})
        for t in DEPRECATED_TAGS.get(name, []):
            if t not in opt["tags_all"]:
                opt["tags_all"].append(t)
        note = f"deprecated {name}->{DEPRECATED[name]}"
        return opt, note

    found = index.find(name)
    if found:
        opt["forms"].append({"kind": "item", "id": found["id"], "name": found["name"]})
        return opt, None

    if name.lower() in EXTRA_FORM_KEYS:
        mapped = EXTRA_FORM_KEYS[name.lower()]
        opt["forms"].append({"kind": "raw", "name": mapped})
        note = f"mapped {name}->{mapped}"
        return opt, note

    base, tier = index.find_tiered(name)
    if base:
        opt["forms"].append({"kind": "item", "id": base["id"], "name": base["name"]})
        opt["tier"] = tier
        note = f"tiered item {name}->{base['name']} +{tier}"
        return opt, note

    return opt, f"unresolved: {name}"


def parse_token(raw: str, index: ItemIndex):
    """Return ('group', group) or ('extra', extra) or ('as', info) or ('error', note)."""
    text = clean(raw)
    if text.startswith("As "):
        m = AS_RE.match(text)
        if m:
            return "as", {"qualifier": m.group(1), "target": m.group(2).strip(), "raw": raw}
        return "error", f"bad As: {raw}"

    qty, qty_max, rest = 1, None, text
    m = QTY_RE.match(text)
    if m:
        qty, qty_max, rest = int(m.group(1)), m.group(2), m.group(3).strip()
        if qty_max:
            qty_max = int(qty_max)

    # currency, with or without the "x" slip
    g = GP_RE.match(rest) or GP_RE.match(text)
    if g:
        amount = int(g.group(1).replace(",", ""))
        return "extra", {"kind": "currency", "quantity": amount, "raw": raw}

    if rest.startswith("Cast the"):
        return "extra", {"kind": "spell", "quantity": qty, "raw": raw}

    if NARRATIVE_RE.search(rest):
        return "extra", {"kind": "narrative", "quantity": qty, "raw": raw}

    min_rarity = None
    token_tier = None
    # Rarity and tier may both prefix the token, in either order
    # ("+2 (Metal) Weapon", "UC (Leather) ...").
    while True:
        m = RARITY_RE.match(rest)
        if m:
            min_rarity, rest = m.group(1), m.group(2).strip()
            continue
        m = TIER_RE.match(rest)
        if m:
            token_tier, rest = int(m.group(1)), m.group(2).strip()
            continue
        break

    # Leading parenthesised qualifiers apply to every alternative in the token.
    global_tags_all, global_tags_any = [], []
    while True:
        lm = LEADING_PAREN_RE.match(rest)
        if not lm:
            break
        for chunk in lm.group(1).split(","):
            chunk = chunk.strip()
            if not chunk:
                continue
            if "/" in chunk:
                for x in chunk.split("/"):
                    x = canon_tag(x)
                    if x and x not in global_tags_any:
                        global_tags_any.append(x)
            else:
                tag = canon_tag(chunk)
                if tag and tag not in global_tags_all:
                    global_tags_all.append(tag)
        rest = rest[lm.end():]

    variable = "of the same trait" in rest or "[Trait]" in rest
    if variable:
        rest = rest.replace("of the same trait", " ")
        rest = rest.replace("[Trait]", " ")
        rest = re.sub(r"[\s()]+", " ", rest).strip()
        if not rest:
            return "group", ({
                "quantity": qty,
                "quantity_max": qty_max,
                "min_rarity": min_rarity,
                "trait_mode": "variable",
                "trait_var": None,
                "options": [new_option()],
            }, [])

    parts = split_alternatives(rest)
    if not parts:
        return "error", f"empty token: {raw}"

    options, notes = [], []
    for part in parts:
        opt, note = classify_part(part, index)
        opt["tags_all"] = list(dict.fromkeys(global_tags_all + opt["tags_all"]))
        opt["tags_any"] = list(dict.fromkeys(global_tags_any + opt["tags_any"]))
        if opt["tier"] is None and token_tier is not None:
            opt["tier"] = token_tier
        normalize_option(opt)
        if note:
            notes.append(note)
        options.append(opt)

    group = {
        "quantity": qty,
        "quantity_max": qty_max,
        "min_rarity": min_rarity,
        "trait_mode": "variable" if variable else "fixed",
        "trait_var": None,
        "options": options,
    }
    return "group", (group, notes)


def parse_alternative(alt: str, index: ItemIndex):
    """Parse one `;`-alternative into (groups, extras, notes, errors)."""
    groups, extras, notes, errors = [], [], [], []
    for token in split_top(alt, ","):
        kind, payload = parse_token(token, index)
        if kind == "group":
            group, token_notes = payload
            groups.append(group)
            for note in token_notes:
                if note.startswith("unresolved") or note == "empty":
                    errors.append(note)
                else:
                    notes.append(note)
        elif kind == "extra":
            extras.append(payload)
        elif kind == "as":
            errors.append(f"inheritance inside an alternative: {token}")
        else:
            errors.append(payload)
    return groups, extras, notes, errors


def dedupe_extras(extras: list[dict]) -> list[dict]:
    out, seen = [], set()
    for e in extras:
        if e["kind"] == "currency":
            sig = (e["kind"], e.get("quantity"))
        else:
            sig = (e["kind"], e.get("quantity"), e["raw"])
        if sig in seen:
            continue
        seen.add(sig)
        out.append(e)
    return out


# ---------------------------------------------------------------------------
# Merging
# ---------------------------------------------------------------------------


def opt_sig(o: dict):
    return (
        o["any_raw"],
        tuple(sorted((f["kind"], f.get("id") or f["name"]) for f in o["forms"])),
        tuple(sorted(o["traits"])),
        tuple(sorted(o["tags_all"])),
        tuple(sorted(o["tags_any"])),
        o["tier"],
    )


def group_sig(g: dict):
    return (
        tuple(sorted(opt_sig(o) for o in g["options"])),
        g["min_rarity"] or "",
        g["trait_mode"],
    )


def dedupe_groups(groups: list[dict], log: list[str]) -> list[dict]:
    """Collapse identical groups.

    A unit counts toward every group it qualifies for, so two identical groups
    are a redundant constraint, not two separate requirements. The stricter
    quantity wins; summing would overcharge the crafter.
    """
    out, seen = [], {}
    for g in groups:
        s = group_sig(g)
        if s in seen:
            prev = seen[s]
            before = (prev["quantity"], prev.get("quantity_max"))
            prev["quantity"] = max(prev["quantity"], g["quantity"])
            if g.get("quantity_max") or prev.get("quantity_max"):
                prev["quantity_max"] = max(
                    prev.get("quantity_max") or prev["quantity"],
                    g.get("quantity_max") or g["quantity"],
                )
            if (prev["quantity"], prev.get("quantity_max")) != before:
                log.append(
                    f"collapsed duplicate group {render_group(g)} -> "
                    f"{prev['quantity']}x"
                )
        else:
            seen[s] = g
            out.append(g)
    for i, g in enumerate(out, 1):
        g["position"] = i
    return out
    for i, g in enumerate(out, 1):
        g["position"] = i
    return out


def merge_alternatives(per_alt: list[list[dict]], log: list[str]):
    """Merge parsed alternatives into a single recipe.

    Returns (groups, conflict_note). conflict_note is None on success.
    """
    sigs = [[group_sig(g) for g in gs] for gs in per_alt]
    if all(s == sigs[0] for s in sigs[1:]):
        return dedupe_groups([copy.deepcopy(g) for g in per_alt[0]], log), None

    common = Counter(sigs[0])
    for s in sigs[1:]:
        common &= Counter(s)

    # Quantities of a group shared by every alternative must agree: a trait
    # amount is a balance decision, not a size approximation.
    qty_conflicts = []
    for s in common:
        qs = {
            g["quantity"]
            for gs in per_alt
            for g in gs
            if group_sig(g) == s
        }
        if len(qs) > 1:
            qty_conflicts.append((s, sorted(qs)))

    common_groups = []
    c = common.copy()
    for g in per_alt[0]:
        s = group_sig(g)
        if c[s] > 0:
            common_groups.append(copy.deepcopy(g))
            c[s] -= 1

    remaining = []
    for gs, ss in zip(per_alt, sigs):
        c = common.copy()
        rem = []
        for g, s in zip(gs, ss):
            if c[s] > 0:
                c[s] -= 1
            else:
                rem.append(g)
        remaining.append(rem)

    if all(len(r) == 0 for r in remaining):
        return dedupe_groups(common_groups, log), None

    if not all(len(r) == 1 for r in remaining):
        sizes = [len(r) for r in remaining]
        return None, f"alternatives differ in more than one group (remaining sizes {sizes})"

    options = []
    for r in remaining:
        options.extend(copy.deepcopy(r[0]["options"]))

    # Drop duplicate options (same alternative repeated inside one group).
    uniq = {}
    for o in options:
        uniq.setdefault(opt_sig(o), o)
    options = list(uniq.values())

    has_traits = any(o["traits"] for o in options)
    qty = remaining[0][0]["quantity"]
    if not has_traits:
        qty = min(r[0]["quantity"] for r in remaining)
    rarities = {r[0]["min_rarity"] for r in remaining}
    if len(rarities) > 1:
        log.append(f"conflicting minimum rarities {sorted(map(str, rarities))}; kept first")

    group = {
        "quantity": qty,
        "quantity_max": None,
        "min_rarity": remaining[0][0]["min_rarity"],
        "trait_mode": remaining[0][0]["trait_mode"],
        "trait_var": None,
        "options": options,
    }
    merged = common_groups + [group]
    groups = dedupe_groups(merged, log)
    note = None
    if qty_conflicts:
        note = f"shared groups had different quantities {qty_conflicts}; kept first"
    return groups, note


# ---------------------------------------------------------------------------
# Post-processing
# ---------------------------------------------------------------------------


def simplify_material_tags(groups: list[dict], log: list[str]) -> None:
    """'(Bone/Metal/Wood) Staff' is just 'Staff': drop universal material tags."""
    universal = {"Bone", "Metal", "Wood"}
    for g in groups:
        forms = None
        same_item = True
        for o in g["options"]:
            if len(o["forms"]) != 1 or o["forms"][0]["kind"] != "item":
                same_item = False
                break
            sig = (o["forms"][0]["kind"], o["forms"][0].get("id") or o["forms"][0]["name"])
            if forms is None:
                forms = sig
            elif forms != sig:
                same_item = False
                break
        if not same_item or forms is None:
            continue

        mat_tags, other_tags = set(), []
        for o in g["options"]:
            for t in o["tags_all"] + o["tags_any"]:
                if t in MATERIAL_TAGS:
                    mat_tags.add(canon_tag(t))
                elif t not in other_tags:
                    other_tags.append(t)
        if not universal <= mat_tags:
            continue

        base = copy.deepcopy(g["options"][0])
        base["tags_all"], base["tags_any"] = [], []
        base["output_tags"], base["output_tags_any"] = [], []
        base["traits"] = []
        for t in other_tags:
            base["tags_all"].append(t)
        log.append(
            f"simplified {render_group(g)} -> {g['quantity']}x {base['forms'][0]['name']}"
        )
        g["options"] = [base]


def material_tag_count(group: dict) -> int:
    tags = set()
    for o in group["options"]:
        for f in o["forms"]:
            if f["kind"] == "raw":
                tags.add(canon_tag(f["name"]))
        for t in o["tags_all"] + o["tags_any"]:
            tag = canon_tag(t)
            if tag in MATERIAL_TAGS:
                tags.add(tag)
    return len(tags)


def apply_output_tags(item_name: str, groups: list[dict], warnings: list[str]) -> None:
    """Give a crafted material unit the tags a later recipe can ask for.

    Only items in TAG_SOURCE_ITEMS are material-like crafted units. Their
    defining group is the one with the richest material vocabulary; e.g. a
    Flask's body (2x Glass/Hide/Bone/Metal), not its metal lid.
    """
    if item_name not in TAG_SOURCE_ITEMS:
        return
    source = max(groups, key=material_tag_count)
    if material_tag_count(source) == 0:
        return
    for o in source["options"]:
        out, out_any = [], []
        for f in o["forms"]:
            if f["kind"] == "raw":
                tag = canon_tag(f["name"])
                if tag not in out:
                    out.append(tag)
            elif f["kind"] == "item" and f.get("id") in MATERIAL_ITEM_IDS:
                tag = MATERIAL_ITEM_IDS[f["id"]]
                if tag not in out:
                    out.append(tag)
        for t in o["tags_all"]:
            tag = canon_tag(t)
            if tag not in out:
                out.append(tag)
        for t in o["tags_any"]:
            tag = canon_tag(t)
            if tag not in out and tag not in out_any:
                out_any.append(tag)
        o["output_tags"] = out
        o["output_tags_any"] = out_any


def dedupe_options(groups: list[dict], item_name: str, log: list[str]) -> None:
    for g in groups:
        seen = {}
        for o in g["options"]:
            normalize_option(o)
            seen.setdefault(opt_sig(o), o)
        if len(seen) != len(g["options"]):
            log.append(f"deduped {len(g['options']) - len(seen)} option(s) in {item_name}")
        g["options"] = list(seen.values())


def collapse_generic_material_options(groups: list[dict], log: list[str]) -> None:
    """Collapse 'any Armor (Hide) / any Armor (Metal)' to 'any Armor'.

    The options are only separated by raw-material tags, so they describe one
    fully generic base rather than distinct choices. Non-material tags (Sword,
    Dragon, ...) still narrow the base and are kept.
    """
    for g in groups:
        options = g["options"]
        if len(options) < 2:
            continue
        first = options[0]
        if len(first["forms"]) != 1 or first["forms"][0]["kind"] != "generic":
            continue
        name = first["forms"][0]["name"]
        tier = first["tier"]
        traits = sorted(first["traits"])
        collapse = True
        for o in options:
            if len(o["forms"]) != 1 or o["forms"][0]["kind"] != "generic":
                collapse = False
                break
            if o["forms"][0]["name"] != name or o["tier"] != tier or sorted(o["traits"]) != traits:
                collapse = False
                break
            for tag in o["tags_all"] + o["tags_any"]:
                if canon_tag(tag) not in RAW_SET:
                    collapse = False
                    break
            if not collapse:
                break
        if not collapse:
            continue
        base = copy.deepcopy(first)
        base["tags_all"], base["tags_any"] = [], []
        g["options"] = [base]
        log.append(f"collapsed material options of {g['quantity']}x any {name} to one generic base")


def detect_modifier(recipe: dict | None) -> dict | None:
    """A recipe that consumes a generic base is a generated modifier combo."""
    if not recipe or recipe["kind"] != "groups":
        return None
    generic = []
    for g in recipe["groups"]:
        for o in g["options"]:
            names = {f["name"] for f in o["forms"] if f["kind"] == "generic"}
            if names:
                generic.append((g, o, names))
    if not generic:
        return None
    # Every generic reference should be in the same group and name.
    names = set()
    for _, _, ns in generic:
        names |= ns
    if len(names) != 1:
        return None
    name = next(iter(names))
    tiers = {o["tier"] for _, o, _ in generic if o["tier"] is not None}
    tier = next(iter(tiers)) if len(tiers) == 1 else None
    return {
        "kind": "modifier",
        "base": {"kind": "category", "name": name},
        "tier": tier,
    }


def validate_groups(item_name: str, groups: list[dict], warnings: list[str]) -> None:
    for g in groups:
        if not g["options"]:
            warnings.append(f"{item_name}: empty requirement group")
        if g["trait_mode"] == "variable":
            continue
        for o in g["options"]:
            if not (o["forms"] or o["traits"] or o["any_raw"]):
                warnings.append(f"{item_name}: option has no form, trait or Material")
            if o["tags_all"] and set(o["tags_all"]) & set(o["tags_any"]):
                warnings.append(f"{item_name}: tag both required and optional")
        if g["trait_mode"] == "variable" and g["trait_var"] is None:
            warnings.append(f"{item_name}: variable trait group is unbound")
        if g["quantity"] > (g["quantity_max"] or g["quantity"]):
            warnings.append(f"{item_name}: quantity range is inverted")


def link_trait_variables(groups: list[dict], item_name: str, warnings: list[str]) -> None:
    for g in groups:
        if g["trait_mode"] != "variable":
            continue
        source = None
        for candidate in groups:
            if candidate is g:
                continue
            for o in candidate["options"]:
                if "Dragon" in o["tags_all"] or "Dragon" in o["tags_any"]:
                    source = candidate
                    break
            if source:
                break
        if source is None:
            warnings.append(f"{item_name}: [Trait] group has no (Dragon) material to bind to")
            g["trait_mode"] = "fixed"
            continue
        g["trait_var"] = source["position"]


def apply_dragon_checks(groups: list[dict], item_name: str, warnings: list[str]) -> None:
    for g in groups:
        for o in g["options"]:
            if "Dragon" not in o["tags_all"] and "Dragon" not in o["tags_any"]:
                continue
            forms = [f for f in o["forms"] if f["kind"] == "raw"]
            allowed = {"Bone", "Hide", "Fluid", "Mineral", "Metal"}
            if forms and any(f["name"] not in allowed for f in forms):
                warnings.append(
                    f"{item_name}: (Dragon) material with form "
                    f"{[f['name'] for f in forms]}"
                )
            if not forms and not o["any_raw"]:
                warnings.append(f"{item_name}: (Dragon) tag on a non-material form")


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------


def form_label(f: dict) -> str:
    return f.get("name", f.get("id", "?"))


def render_option(o: dict) -> str:
    if o["any_raw"]:
        base = "Material"
    else:
        base = "/".join(form_label(f) for f in o["forms"]) or ""
    if o["tier"]:
        base = f"+{o['tier']} {base}"
    if o["traits"]:
        base = "/".join(o["traits"]) + (" " + base if base else "")
    if o["tags_all"]:
        base += " (" + ", ".join(o["tags_all"]) + ")"
    if o["tags_any"]:
        base += " (" + "/".join(o["tags_any"]) + ")"
    return base.strip()


def render_group(g: dict) -> str:
    q = f"{g['quantity']}x"
    if g["quantity_max"]:
        q = f"{g['quantity']}-{g['quantity_max']}x"
    if g["trait_mode"] == "variable":
        body = "[trait]"
    else:
        body = " | ".join(render_option(o) for o in g["options"])
    if g["min_rarity"]:
        body = f"{g['min_rarity']} {body}"
    return f"{q} {body}"


def render_recipe(recipe: dict | None) -> str:
    if not recipe:
        return "(no recipe)"
    if recipe["kind"] == "inherit":
        sel = recipe["base_selector"]
        return f"As {sel['name']}"
    parts = [render_group(g) for g in recipe["groups"]]
    for e in recipe.get("extras", []):
        if e["kind"] == "currency":
            parts.append(f"{e['quantity']} GP")
        elif e["kind"] == "spell":
            parts.append("Cast a spell")
        else:
            parts.append("narrative")
    return "; ".join(parts) if parts else "(empty)"


# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------


def load_items() -> list[dict]:
    rows = []
    for path in (DATA / "items.tsv", DATA / "items_extra.tsv"):
        if not path.exists():
            continue
        with path.open(newline="") as fh:
            rows.extend(csv.DictReader(fh, delimiter="\t"))
    return rows


def add_synthetic_tier_templates(items: list[dict]) -> None:
    """Add Ammunition +1..+4 next to the Weapon/Armor tier templates."""
    existing = {r["name"] for r in items}
    for tier, rarity in ((1, "Uncommon"), (2, "Rare"), (3, "Very Rare"), (4, "Legendary")):
        name = f"Ammunition +{tier}"
        if name in existing:
            continue
        items.append({
            "id": f"ammunition-plus-{tier}",
            "name": name,
            "rarity": rarity,
            "source": "Crafting",
            "magic": "TRUE",
            "visible": "TRUE",
            "type": "Ammunition",
            "recipes": "As Ammunition",
            "short_description": f"Tier {tier} template for any mundane ammunition.",
            "scraped_name": name,
            "match_status": "needs_review",
            "match_method": "generated",
            "match_detail": "synthetic_tier",
            "match_confidence": "1.0",
            "scraped_source_code": "",
            "scraped_source": "Crafting",
            "scraped_rarity": rarity,
            "scraped_type": "Ammunition",
            "attunement": "",
            "damage": "",
            "properties": "",
            "weight": "",
            "value": "",
            "scraped_text": "",
        })


def split_type(type_raw: str):
    head, _, tail = type_raw.partition(",")
    return head.strip(), (tail.strip() or None)


def build() -> int:
    items = load_items()
    add_synthetic_tier_templates(items)
    index = ItemIndex(items)
    for r in items:
        r["_extra"] = r.get("id", "").startswith(("paper", "parchment"))
        source = RECIPE_OVERRIDES.get(r["name"], r.get("recipes") or "")
        alts = source if isinstance(source, list) else source.split(";")
        r["_alts"] = [a.strip() for a in alts if a.strip()]
        r["_recipe"] = None
        r["_notes"] = []
        r["_warnings"] = []
        r["_conflict"] = None
        r["_archetype"] = None

    # pass 1: recipes without inheritance
    for r in items:
        if any(a.startswith("As ") for a in r["_alts"]):
            continue
        if not r["_alts"]:
            continue
        per_alt, extras, notes = [], [], []
        for alt in r["_alts"]:
            groups, alt_extras, alt_notes, errors = parse_alternative(alt, index)
            if errors:
                r["_warnings"].extend(f"parse: {e}" for e in errors)
            per_alt.append(groups)
            extras.extend(alt_extras)
            notes.extend(alt_notes)
        if not any(per_alt):
            r["_warnings"].append("recipe has no material requirement")
            continue
        groups, conflict = merge_alternatives(per_alt, r["_notes"])
        if conflict:
            r["_conflict"] = conflict
            continue
        r["_recipe"] = {
            "kind": "groups",
            "rarity": r.get("rarity") or "",
            "extras": dedupe_extras(extras),
            "groups": groups,
            "notes": notes,
        }

    # pass 2: inheritance
    for r in items:
        as_alts = [a for a in r["_alts"] if a.startswith("As ")]
        if not as_alts:
            continue
        if len(as_alts) != len(r["_alts"]):
            r["_warnings"].append("mixes As and explicit alternatives; using As only")
        per_alt, extras, notes = [], [], []
        archetype = None
        for alt in as_alts:
            kind, payload = parse_token(alt, index)
            if kind != "as":
                r["_warnings"].append(f"bad As: {alt}")
                continue
            target = payload["target"]
            base = index.find(target)
            if base is None and target in GENERIC_FORMS:
                archetype = {"name": target, "kind": "category"}
                continue
            if base is None:
                r["_warnings"].append(f"unresolved As target: {target}")
                continue
            if not base["_recipe"] or base["_recipe"]["kind"] != "groups":
                r["_warnings"].append(f"As target {target} has no recipe")
                continue
            groups = copy.deepcopy(base["_recipe"]["groups"])
            qualifier = payload["qualifier"]
            if qualifier:
                qnames = {
                    DEPRECATED.get(x.strip(), x.strip())
                    for x in split_alternatives(qualifier)
                }
                for g in groups:
                    matched = [
                        o for o in g["options"]
                        if any(form_label(f) in qnames for f in o["forms"])
                        or any(t in qnames for t in o["tags_all"] + o["tags_any"])
                    ]
                    if matched:
                        g["options"] = matched
            per_alt.append(groups)

        if archetype is not None:
            r["_archetype"] = {
                "kind": "tier",
                "base": {"kind": archetype["kind"], "name": archetype["name"]},
                "tier": int((re.search(r"\+(\d)", r["name"]) or [0, 0])[1]) or None,
            }
            r["_recipe"] = {
                "kind": "inherit",
                "rarity": r.get("rarity") or "",
                "base_selector": {"kind": archetype["kind"], "name": archetype["name"]},
                "extras": [],
                "groups": [],
                "notes": [],
            }
            continue

        if not per_alt:
            continue
        groups, conflict = merge_alternatives(per_alt, r["_notes"])
        if conflict:
            r["_conflict"] = conflict
            continue
        r["_recipe"] = {
            "kind": "groups",
            "rarity": r.get("rarity") or "",
            "extras": dedupe_extras(extras),
            "groups": groups,
            "notes": notes,
        }

    # post-process
    for r in items:
        r["_template"] = bool(
            re.fullmatch(r"(Weapon|Armor|Ammunition) \+\d", r["name"])
            or r["name"] == "Vicious Weapon"
        )
        if not r["_recipe"] or r["_recipe"]["kind"] != "groups":
            continue
        groups = r["_recipe"]["groups"]
        simplify_material_tags(groups, r["_notes"])
        dedupe_options(groups, r["name"], r["_notes"])
        collapse_generic_material_options(groups, r["_notes"])
        for i, g in enumerate(groups, 1):
            g["position"] = i
        link_trait_variables(groups, r["name"], r["_warnings"])
        apply_output_tags(r["name"], groups, r["_warnings"])
        apply_dragon_checks(groups, r["name"], r["_warnings"])
        validate_groups(r["name"], groups, r["_warnings"])
        if r["_archetype"] is None:
            r["_archetype"] = detect_modifier(r["_recipe"])

    # serialize
    OUT.mkdir(parents=True, exist_ok=True)
    records = []
    for r in items:
        category, subtype = split_type(r.get("type", ""))
        short = (r.get("short_description") or "").strip()
        status = (r.get("match_status") or "").strip() or "needs_review"
        variant_of, variant_label = None, None
        if (r.get("match_method") or "") == "variant_base" and ", " in r["name"]:
            variant_of = r.get("scraped_name") or r["name"].split(", ", 1)[0]
            variant_label = r["name"].split(", ", 1)[1].strip()
        rec = {
            "id": r["id"],
            "name": r["name"],
            "category": category,
            "subtype": subtype,
            "type_raw": r.get("type", ""),
            "rarity": r.get("rarity", ""),
            "magic": (r.get("magic") or "").upper() == "TRUE",
            "visible": (r.get("visible") or r.get("oota") or "").upper() == "TRUE",
            "source": r.get("source", ""),
            "short_description": None if short in ("", "-") else short,
            "variant_of": variant_of,
            "variant_label": variant_label,
            "attunement": r.get("attunement") or None,
            "damage": r.get("damage") or None,
            "properties": r.get("properties") or None,
            "weight": r.get("weight") or None,
            "value": r.get("value") or None,
            "description": r.get("scraped_text") or None,
            "scraped_name": r.get("scraped_name") or None,
            "stat_source_code": r.get("scraped_source_code") or None,
            "stat_source": r.get("scraped_source") or None,
            "match_status": status,
            "match_method": r.get("match_method") or None,
            "match_detail": r.get("match_detail") or None,
            "match_confidence": float(r["match_confidence"]) if r.get("match_confidence") else None,
            "intrinsic_tags": item_intrinsic_tags(r["name"], index),
            "archetype": r["_archetype"],
            "template": r.get("_template", False),
            "recipe": r["_recipe"],
            "warnings": r["_warnings"],
        }
        records.append(rec)

    with (OUT / "items.jsonl").open("w") as fh:
        for rec in records:
            fh.write(json.dumps(rec, ensure_ascii=False) + "\n")

    with (OUT / "recipes.txt").open("w") as fh:
        for rec in records:
            fh.write(f"{rec['name']}\t{render_recipe(rec['recipe'])}\n")

    write_report(items, records)
    print_summary(items, records)
    return 0


def write_report(items: list[dict], records: list[dict]) -> None:
    lines = ["# Dataset build report", ""]
    merged, simplified, warnings, conflicts = [], [], [], []
    for r in items:
        if r["_notes"]:
            merged.append((r["name"], r["_notes"]))
        if r["_conflict"]:
            conflicts.append((r["name"], r["_conflict"]))
        for w in r["_warnings"]:
            warnings.append((r["name"], w))
    for r in records:
        merged.extend([])
    # simplification notes live in _notes with a marker
    lines.append(f"- items: {len(records)}")
    lines.append(f"- with a recipe: {sum(1 for r in records if r['recipe'])}")
    lines.append(f"- recipes that inherit a base: {sum(1 for r in records if r['recipe'] and r['recipe']['kind'] == 'inherit')}")
    lines.append(f"- archetypes: {sum(1 for r in records if r['archetype'])}")
    lines.append(f"- conflicts: {len(conflicts)}")
    lines.append(f"- warnings: {len(warnings)}")
    lines.append("")
    if conflicts:
        lines += ["## Conflicts (no recipe emitted)", ""]
        lines += [f"- **{n}**: {c}" for n, c in conflicts]
        lines.append("")
    if warnings:
        lines += ["## Warnings", ""]
        lines += [f"- {n}: {w}" for n, w in warnings]
        lines.append("")
    lines += ["## Merge / normalization notes", ""]
    for n, notes in merged:
        lines += [f"- {n}: {x}" for x in notes]
    (OUT / "report.md").write_text("\n".join(lines) + "\n")


def print_summary(items: list[dict], records: list[dict]) -> None:
    groups = sum(len(r["recipe"]["groups"]) for r in records if r["recipe"] and r["recipe"]["kind"] == "groups")
    options = sum(len(g["options"]) for r in records if r["recipe"] and r["recipe"]["kind"] == "groups" for g in r["recipe"]["groups"])
    print(f"items: {len(records)}")
    print(f"recipes: {sum(1 for r in records if r['recipe'])} "
          f"({sum(1 for r in records if r['recipe'] and r['recipe']['kind']=='inherit')} inherit, "
          f"{sum(1 for r in records if not r['recipe'])} missing)")
    print(f"requirement groups: {groups}  options: {options}")
    print(f"archetypes: {sum(1 for r in records if r['archetype'])}")
    print(f"conflicts: {sum(1 for r in items if r['_conflict'])}")
    print(f"warnings: {sum(len(r['_warnings']) for r in items)}")
    print(f"wrote {OUT / 'items.jsonl'}, {OUT / 'recipes.txt'}, {OUT / 'report.md'}")


if __name__ == "__main__":
    raise SystemExit(build())
