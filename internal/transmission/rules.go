package transmission

import "regexp"

type routeRule struct {
	name        string
	pattern     *regexp.Regexp
	destination string
}

// The first match wins. An empty destination keeps the torrent in place.
var defaultRules = [...]routeRule{
	{
		name:        "plus-belle-la-vie",
		pattern:     regexp.MustCompile(`(?i)^Plus[.]belle[.]la[.]vie[.]encore[.]plus[.]belle[.]S03E[0-9]{3}([ ._-]|$)`),
		destination: "/mnt/sdb/mylen/Plus.belle.la.vie.encore.plus.belle/s03",
	},
	{
		name:        "series",
		pattern:     regexp.MustCompile(`(?i)(^|[ ._-])(s[0-9]{1,2}(e[0-9]{1,3})?|[0-9]{1,2}x[0-9]{1,3})([ ._-]|$)`),
		destination: "/mnt/sda/series",
	},
	{
		name:        "movies",
		pattern:     regexp.MustCompile(`(^|[ ._(])(19|20)[0-9]{2}([ ._)]|$)`),
		destination: "/mnt/sdb/movies-hd",
	},
}

func matchRoute(rules []routeRule, name string) (routeRule, bool) {
	for _, rule := range rules {
		if rule.pattern.MatchString(name) {
			return rule, true
		}
	}
	return routeRule{}, false
}
