package auth

import (
	"strconv"
	"time"
)

// parseRealtime converts journald's __REALTIME_TIMESTAMP (microseconds
// since epoch, as a string) into a time.Time. This is the only piece of
// the old parser that survives the move to server-side decoding — every
// raw record still needs a real timestamp, regardless of who interprets
// its meaning.
func parseRealtime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	micros, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMicro(micros).UTC()
}
