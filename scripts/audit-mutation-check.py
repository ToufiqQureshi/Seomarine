#!/usr/bin/env python3
"""Mutation check for backend/internal/audit.

Applies one deliberate defect at a time, runs the package tests, and reports
whether a test caught it. A surviving mutation means a missing test.

Usage:
    python3 scripts/audit-mutation-check.py [package-path]

Run from the repository root; the Go module lives in backend/.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BACKEND = os.path.join(REPO_ROOT, "backend")
PKG = "./internal/audit"

# (path relative to backend/, original snippet, mutated snippet, description)
MUTATIONS: list[tuple[str, str, str, str]] = [
    ("internal/audit/urlpolicy.go",
     "return isPrivateIPv4(host) || isPrivateIPv6(host)",
     "return false",
     "IP-literal blocking disabled (SSRF)"),
    ("internal/audit/urlpolicy.go",
     "\tcase a == 10, a == 127, a == 0:",
     "\tcase a == 10, a == 999, a == 0:",
     "loopback range no longer private"),
    ("internal/audit/urlpolicy.go",
     "\t\tif !isPublicAddr(addr) {\n\t\t\treturn nil, fmt.Errorf(\"%w: %s resolves to %s\", ErrCrawlTargetBlocked, host, addr)\n\t\t}\n\t\tallowed = append(allowed, addr)",
     "\t\tallowed = append(allowed, addr)",
     "one private DNS answer no longer blocks the host"),
    ("internal/audit/classifyfetch.go",
     "\tif statusCode == 429 {\n\t\treturn FetchRateLimited\n\t}",
     "\tif statusCode == 999 {\n\t\treturn FetchRateLimited\n\t}",
     "429 no longer classified as rate limited"),
    ("internal/audit/crawlwindow.go",
     "\tif troubled*3 >= len(recent) {",
     "\tif troubled*3 >= len(recent)*100 {",
     "window no longer shrinks under stress"),
    ("internal/audit/crawlthrottle.go",
     "\t\tt.state.ConsecutiveRateLimits > throttleMaxRetries ||",
     "\t\tt.state.ConsecutiveRateLimits > 100 ||",
     "throttle never stops after repeated 429s"),
    ("internal/audit/pageanalyzer.go",
     "wordCount < 20 &&",
     "wordCount < 0 &&",
     "javascript shell never detected"),
    ("internal/audit/pagereporters.go",
     "\tif page.ImagesMissingAlt > 0 {",
     "\tif page.ImagesMissingAlt >= 0 {",
     "missing-alt issue always reported"),
    ("internal/audit/pagereporters.go",
     "\tcase len(page.Title) > titleMaxChars:",
     "\tcase len(page.Title) > 1000:",
     "long titles never flagged"),
    ("internal/audit/multipage.go",
     "\treturn effectiveCanonical == \"\" || effectiveCanonical == page.URL",
     "\treturn true",
     "canonicalized pages still grouped as duplicates"),
    ("internal/audit/robots.go",
     "\t\treturn true\n\t}\n\treturn bestAllow\n}",
     "\t\treturn true\n\t}\n\treturn true\n}",
     "robots disallow rules ignored"),
    ("internal/audit/sitemap.go",
     "strings.HasPrefix(trimmed, \"<urlset\") ||",
     "false ||",
     "urlset no longer recognized as a sitemap"),
    ("internal/audit/lighthouse.go",
     "if page.StatusCode >= 200 && page.StatusCode < 300 && page.FetchClass == FetchOK {",
     "if true {",
     "lighthouse samples blocked/error pages"),
    ("internal/audit/limits.go",
     "\t\tlighthouseChecks = 20",
     "\t\tlighthouseChecks = 0",
     "lighthouse capacity reservation dropped"),
    ("internal/audit/types.go",
     "\t\t\tcase string(LighthouseNone), \"manual\":",
     "\t\t\tcase string(LighthouseNone):",
     "retired 'manual' strategy no longer mapped"),
    ("internal/audit/auditerrors.go",
     "\t\treturn ErrorCPULimit",
     "\t\treturn ErrorOOM",
     "CPU-limit errors misclassified"),
    ("internal/audit/ids.go",
     "\treturn digest[:36]",
     "\treturn digest[:35]",
     "deterministic id truncated to the wrong length"),
    ("internal/audit/crawler.go",
     "if token == \"noindex\" || token == \"none\" {",
     "if token == \"noindexX\" || token == \"none\" {",
     "noindex no longer recognized"),
    ("internal/audit/handler.go",
     "\tdecoder.DisallowUnknownFields()",
     "",
     "unknown JSON fields accepted"),
    ("internal/audit/repository.go",
     "\tinternal, external := 0, 0",
     "\tinternal, external := 1, 1",
     "link counts seeded wrongly"),
]


def run_tests() -> bool:
    """Return True when the package tests pass."""
    result = subprocess.run(
        ["go", "test", PKG, "-count=1"],
        cwd=BACKEND,
        capture_output=True,
        text=True,
    )
    return result.returncode == 0


def main() -> int:
    if not run_tests():
        print("baseline tests fail; fix them before mutating", file=sys.stderr)
        return 2

    caught = 0
    survived: list[str] = []
    for relative, original, mutated, description in MUTATIONS:
        path = os.path.join(BACKEND, relative)
        with open(path, "r", encoding="utf-8", newline="") as handle:
            content = handle.read()
        if original not in content:
            print(f"SKIP  {description}: anchor not found in {relative}")
            continue
        with tempfile.NamedTemporaryFile("w", delete=False, encoding="utf-8", newline="") as backup:
            backup.write(content)
            backup_path = backup.name
        try:
            with open(path, "w", encoding="utf-8", newline="") as handle:
                handle.write(content.replace(original, mutated, 1))
            if run_tests():
                survived.append(f"{description} ({relative})")
                print(f"SURVIVED  {description}")
            else:
                caught += 1
                print(f"caught    {description}")
        finally:
            shutil.copyfile(backup_path, path)
            os.unlink(backup_path)

    print(f"\nmutations caught: {caught}, survived: {len(survived)}")
    for description in survived:
        print(f"  survivor: {description}")
    return 1 if survived else 0


if __name__ == "__main__":
    raise SystemExit(main())
