package utils

import "math"

func CopyMap(src map[string]string) map[string]string {
	des := map[string]string{}
	for k, v := range src {
		des[k] = v
	}
	return des
}

// IncludeNonEmpty inserts (and overwrites) data into map object using specified key, if not empty value provided
func IncludeNonEmpty(dst map[string]string, key, src string) {
	// Do not include empty value
	if src == "" {
		return
	}

	// Include (and overwrite) value by specified key
	dst[key] = src

	return
}

func ChangeBToMBWithJVMRatio(memory int64) int {
	return int(math.Round((float64(memory) * JVMRatio) / (1024 * 1024)))
}
