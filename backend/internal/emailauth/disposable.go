package emailauth

import "strings"

// disposableDomains is a curated blocklist of the highest-volume throwaway
// inbox providers. The free plan grants real credit spend to a verified email,
// so the cheapest way to farm it is a temp inbox that can still receive the
// verification link. It is deliberately small and not exhaustive.
var disposableDomains = map[string]struct{}{
	"0clock.net": {}, "10minutemail.com": {}, "20minutemail.com": {}, "33mail.com": {},
	"burnermail.io": {}, "dispostable.com": {}, "emailondeck.com": {}, "fakeinbox.com": {},
	"getairmail.com": {}, "getnada.com": {}, "guerrillamail.com": {}, "guerrillamail.net": {},
	"guerrillamail.org": {}, "inboxbear.com": {}, "inboxkitten.com": {}, "mail-temp.com": {},
	"mailcatch.com": {}, "maildrop.cc": {}, "mailinator.com": {}, "mailnesia.com": {},
	"moakt.com": {}, "mohmal.com": {}, "mytemp.email": {}, "sharklasers.com": {},
	"spam4.me": {}, "temp-mail.io": {}, "temp-mail.org": {}, "tempmail.com": {},
	"tempmail.dev": {}, "tempmailo.com": {}, "tempr.email": {}, "throwawaymail.com": {},
	"trashmail.com": {}, "trashmail.de": {}, "yopmail.com": {}, "yopmail.fr": {}, "yopmail.net": {},
}

func isDisposableEmail(address string) bool {
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return false
	}
	_, blocked := disposableDomains[strings.ToLower(strings.TrimSpace(address[at+1:]))]
	return blocked
}
