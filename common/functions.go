package common

import "time"

// PartialToFullIso completes a partial ISO datetime. The partial can be:
// * A date in YYYY-MM-DD format (00:00:00.000 UTC will be assumed)
// * A timestamp in YYYY-MM-DDTHH:MM:SS format (UTC will be assumed)
// * A timestamp in YYYY-MM-DDTHH:MM:SS.MMM format (UTC will be assumed)
// The format selection is very naive, it uses only the length
func PartialToFullIso(partial string) string {
	switch len(partial) {
	case 10: // YYYY-MM-DD
		return partial + "T00:00:00.000-0000"
	case 19: // YYYY-MM-DDTHH:MM:SS
		return partial + ".000-0000"
	case 23: // YYYY-MM-DDTHH:MM:SS.000
		return partial + "-0000"
	}

	// In all other cases, assume it's already full ISO
	return partial
}

// ParsePartialIsoTime parses a partial ISO timestamp, according to the partialToFullIso function
func ParsePartialIsoTime(ts string) (time.Time, error) {
	t := PartialToFullIso(ts)
	return time.Parse(ISO8601_FORMAT, t)
}
