// Command stubauth stands in for Supabase's GET /auth/v1/user on localhost for the scale test.
// A bearer token "scale1m-learner-<n>" (1 <= n <= 1,000,000, zero-padded to 7 digits because
// the API rejects tokens under 20 characters) is the learner whose id load.sql gave as
// md5('scale1m-' || n)::uuid. Anything else is a 401, like an expired token.
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const prefix, maxLearner = "scale1m-learner-", 1_000_000

func learnerID(token string) (string, bool) {
	s, ok := strings.CutPrefix(token, prefix)
	n, err := strconv.Atoi(s)
	if !ok || err != nil || n < 1 || n > maxLearner {
		return "", false
	}
	h := hex.EncodeToString(func(b [16]byte) []byte { return b[:] }(md5.Sum([]byte("scale1m-" + strconv.Itoa(n)))))
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], true
}

func main() {
	addr := flag.String("addr", "127.0.0.1:5545", "listen address")
	flag.Parse()
	// Same answer Postgres gives for md5('scale1m-1')::uuid.
	if id, _ := learnerID(prefix + "0000001"); id != "ba328e06-7e21-2ad9-8e5a-a715a711b60e" {
		log.Fatalf("learnerID self-check failed: %s", id)
	}
	http.HandleFunc("GET /auth/v1/user", func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		id, ok := learnerID(strings.TrimSpace(token))
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"code":401,"error_code":"bad_jwt","msg":"invalid JWT"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id": id, "aud": "authenticated", "role": "authenticated"})
	})
	log.Printf("stub auth on http://%s/auth/v1/user", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
