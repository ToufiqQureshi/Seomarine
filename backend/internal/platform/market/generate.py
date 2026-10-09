"""Regenerate data.json from the legacy market table during the migration."""

import json
import re
from pathlib import Path

root = Path(__file__).resolve().parents[4]
source = (root / "src/shared/keyword-locations.ts").read_text(encoding="utf-8")


def section(start: str, end: str) -> str:
    return source.split(start, 1)[1].split(end, 1)[0]


def records(block: str, keys: tuple[str, ...]) -> list[dict]:
    result = []
    for match in re.finditer(r"\{([^{}]+)\}", block):
        body = match.group(1)
        values = {}
        for key in keys:
            value = re.search(rf"\b{key}:\s*(\d+|true|\"[^\"]*\")", body)
            if value:
                values[key] = json.loads(value.group(1))
        if all(key in values for key in keys):
            values["googleAdsOnly"] = bool(re.search(r"\bgoogleAdsOnly:\s*true", body))
            result.append(values)
    return result


locations = records(
    section("export const LOCATION_OPTIONS:", "export const SERP_LANGUAGE_OPTIONS"),
    ("code", "label", "shortLabel", "languageCode"),
)
languages = records(
    section("export const SERP_LANGUAGE_OPTIONS = [", "] as const;"),
    ("code", "label"),
)
multi = {}
for code, values in re.findall(
    r"(?m)^\s*(\d+):\s*(\[[^\]]+\])",
    section("const MULTI_LANGUAGE_LOCATIONS:", "export function getLanguageOptions"),
):
    multi[code] = json.loads(values)

if len(locations) < 100 or len(languages) < 100 or len(multi) < 10:
    raise SystemExit("market table parse incomplete")
target = Path(__file__).with_name("data.json")
target.write_text(
    json.dumps({"locations": locations, "languages": languages, "multi": multi}, ensure_ascii=False, separators=(",", ":")) + "\n",
    encoding="utf-8",
)
print(f"{len(locations)} locations, {len(languages)} languages, {len(multi)} multilingual locations")
