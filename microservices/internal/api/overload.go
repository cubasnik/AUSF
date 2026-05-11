package api

import (
	"net/http"
	"strconv"
	"sync/atomic"
)

// withOverloadControl inserts a load-shedding layer mandated by TS 29.500 §6.5
// NF Overload Control.  It counts in-flight requests atomically and rejects any
// request that arrives when the count already equals threshold with HTTP 503
// plus the required 3GPP SBI headers:
//
//   - 3gpp-Sbi-Overload-Control  — current concurrency, sent on every response
//   - Retry-After: 5             — backoff hint, sent only on 503 responses
//   - 3gpp-Sbi-Max-Rsp-Time: 5000 — ms budget hint, sent only on 503 responses
//
// threshold ≤ 0 uses the default value of 500.
func withOverloadControl(next http.Handler, threshold int) http.Handler {
	if threshold <= 0 {
		threshold = 500
	}
	var inflight int64
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&inflight, 1)
		defer atomic.AddInt64(&inflight, -1)

		// Advertise current concurrency on every response so consumers can
		// adapt their sending rate per TS 29.500 §6.5.3.
		w.Header().Set("3gpp-Sbi-Overload-Control", strconv.FormatInt(cur, 10))

		if cur > int64(threshold) {
			w.Header().Set("Retry-After", "5")
			w.Header().Set("3gpp-Sbi-Max-Rsp-Time", "5000")
			writeProblem(w, http.StatusServiceUnavailable,
				"Service overloaded",
				"too many concurrent requests; retry later",
				"NF_CONGESTION",
				r.URL.Path,
			)
			return
		}
		next.ServeHTTP(w, r)
	})
}
