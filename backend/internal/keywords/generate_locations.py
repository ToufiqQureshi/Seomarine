"""Copy SERP search abbreviation and type-rank tables from the legacy UI."""

import json
import re
from pathlib import Path

root = Path(__file__).resolve().parents[3]
source = (root / "src/shared/serp-location-search.ts").read_text(encoding="utf-8")
section = source.split("export const REGION_ABBREVIATIONS:", 1)[1].split("export const LOCATION_TYPE_RANK:", 1)[0]
abbreviations = {}
for country, body in re.findall(r"(?m)^\s*(us|ca|au):\s*\{([^{}]+)\}", section):
    abbreviations[country] = dict(re.findall(r'\b([a-z]+):\s*"([^"]+)"', body))
rank_section = source.split("export const LOCATION_TYPE_RANK:", 1)[1].split("};", 1)[0]
ranks = {}
for quoted, bare, rank in re.findall(r'(?m)^\s*(?:"([^"]+)"|([A-Za-z]+)):\s*(\d+)', rank_section):
    ranks[quoted or bare] = int(rank)
if len(abbreviations) != 3 or len(abbreviations["us"]) < 50 or len(ranks) < 10:
    raise SystemExit("incomplete SERP location search tables")
Path(__file__).with_name("location_search.json").write_text(
    json.dumps({"abbreviations": abbreviations, "typeRanks": ranks}, ensure_ascii=False, separators=(",", ":")) + "\n",
    encoding="utf-8",
)
print(f"{sum(map(len, abbreviations.values()))} abbreviations, {len(ranks)} type ranks")
