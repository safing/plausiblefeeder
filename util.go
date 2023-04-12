package plausiblefeeder

// Utils, so we don't have any dependencies to make it easier for the traefik plugin.

func sliceContainsString(s []string, a string) bool {
	for _, v := range s {
		if v == a {
			return true
		}
	}
	return false
}
