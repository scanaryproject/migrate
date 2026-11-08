package migrate

import (
	nurl "net/url"
)

// FilterCustomQuery filters all query values starting with `x-`
func FilterCustomQuery(u *nurl.URL) *nurl.URL {
	ux := *u
	vx := make(nurl.Values)
	for k, v := range ux.Query() {
		if len(k) <= 1 || k[0:2] != "x-" {
			vx[k] = v
		}
	}
	ux.RawQuery = vx.Encode()
	return &ux
}
