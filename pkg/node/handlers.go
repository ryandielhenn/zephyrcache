package node

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

// healthz returns 200 OK to indicate the Node is alive.
func (s *Node) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// info writes a JSON payload with the process ID, current time, and KV item count.
func (s *Node) Info(w http.ResponseWriter, _ *http.Request) {
	type resp struct {
		PID   int       `json:"pid"`
		Now   time.Time `json:"now"`
		Items int       `json:"items"`
	}
	data, _ := json.Marshal(resp{PID: os.Getpid(), Now: time.Now(), Items: s.kv.Len()})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// forward forwards a http request to the Node that owns the key
func (s *Node) Forward(w http.ResponseWriter, req *http.Request, owner string) {
	if owner == "" {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	hostport := NormalizeHostPort(owner, "8080")
	if NormalizeHostPort(s.addr, "8080") == hostport {
		// last-resort safety; shouldn’t happen if handler compare is correct
		http.Error(w, "refusing to forward to self", http.StatusInternalServerError)
		return
	}
	target := *req.URL
	target.Scheme = "http"
	target.Host = hostport

	out, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	out.Header = req.Header.Clone()

	out.Header.Set("X-Forwarded-For", req.RemoteAddr)

	resp, err := http.DefaultClient.Do(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

}

// put adds a key/value pair
func (n *Node) Put(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	owner, self, ok := n.OwnerForKey(key)
	if !ok {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	if owner != self {
		log.Printf("[Forward PUT] key=%q owner=%q self=%q", key, owner, self)
		n.Forward(w, req, owner)
		return
	}

	// handle local case
	val, err := io.ReadAll(req.Body)
	if err != nil && err.Error() != "EOF" {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var ttl time.Duration
	if ttlStr := req.URL.Query().Get("ttl"); ttlStr != "" {
		sec, err := strconv.Atoi(ttlStr)
		if err != nil {
			http.Error(w, "invalid ttl", http.StatusBadRequest)
			return
		}
		ttl = time.Duration(sec) * time.Second
	}
	n.kv.Put(key, val, ttl)
	w.WriteHeader(http.StatusNoContent)
}

// get returns the value for a key
func (n *Node) Get(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	owner, self, ok := n.OwnerForKey(key)
	if !ok {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	if owner != self {
		log.Printf("[Forward GET] key=%q owner=%q self=%q", key, owner, self)
		n.Forward(w, req, owner)
		return
	}

	// handle local case
	val, ok := n.kv.Get(key)
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(val)
}

// del removes a key
func (n *Node) Del(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	owner, self, ok := n.OwnerForKey(key)
	if !ok {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	if owner != self {
		log.Printf("[Forward DEL] key=%q owner=%q self=%q", key, owner, self)
		n.Forward(w, req, owner)
		return
	}

	// handle local case
	n.kv.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}
