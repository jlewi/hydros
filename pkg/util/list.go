package util

// UniqueStrings takes a list of strings and returns a list with unique strings.
func UniqueStrings(strings []string) []string {
	stringSet := map[string]bool{}
	for _, str := range strings {
		if _, ok := stringSet[str]; !ok {
			stringSet[str] = true
		}
	}
	uniqueStrings := make([]string, len(stringSet))
	i := 0
	for str := range stringSet {
		uniqueStrings[i] = str
		i++
	}
	return uniqueStrings
}
