/**
 * The iframe's `sandbox` attribute value, matching the CSP's sandbox
 * directive. `allow-popups` plus `allow-popups-to-escape-sandbox` let a
 * report's `target="_blank"` links open normally (the audited site, a
 * reference); everything else stays restricted. The attribute must always be
 * present — an absent `sandbox` attribute is no sandbox at all — and
 * `allow-same-origin` is never added: it would hand the frame the app's cookies
 * and, with scripts, let the frame remove its own sandbox.
 */
export const REPORT_IFRAME_SANDBOX =
  "allow-popups allow-popups-to-escape-sandbox";

const CSP_TAIL =
  "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'self'";

// The report's own JavaScript is blocked, and there is no `script-src`:
// `default-src 'none'` already blocks every script. One flip turns it on, in
// three places: add `allow-scripts` to the sandbox directive below, add it to
// REPORT_IFRAME_SANDBOX, and add `script-src 'unsafe-inline'` here. Reports
// saved while JS is blocked keep working after the flip; the reverse is not
// true, which is why blocking is the V1 default.
const REPORT_CSP = `sandbox ${REPORT_IFRAME_SANDBOX}; ${CSP_TAIL}`;

/**
 * The only script authorized in print mode. It opens closed `<details>` before
 * the browser snapshots the page for printing.
 */
export const PRINT_SCRIPT =
  'addEventListener("load",()=>{for(const d of document.querySelectorAll("details"))d.open=true;setTimeout(()=>print(),150)})';

/**
 * Base64 SHA-256 of PRINT_SCRIPT, for the `script-src` hash source. Hardcoded
 * rather than computed at module load: the digest is needed synchronously and
 * Web Crypto is async. report-sandbox.test.ts recomputes it from PRINT_SCRIPT,
 * so editing the script without editing this constant fails the suite instead
 * of silently breaking printing.
 */
export const PRINT_SCRIPT_SHA256 =
  "uE0MLbgBgIu3I3pghlY0762kVHKlP7Udz5qzZNT7piw=";

/**
 * The policy for a report document. Print mode permits only PRINT_SCRIPT.
 */
export function reportCsp(print?: boolean): string {
  if (!print) return REPORT_CSP;
  return `sandbox ${REPORT_IFRAME_SANDBOX} allow-scripts allow-modals; script-src 'sha256-${PRINT_SCRIPT_SHA256}'; ${CSP_TAIL}`;
}
