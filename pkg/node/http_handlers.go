package node

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// healthz returns 200 OK to indicate the Node is alive.
func (s *Node) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("ok"))
	if err != nil {
		slog.Warn("Could not write http response header for healthcheck request")
	}
}

// info writes a JSON payload with the process ID, current time, and KV item count.
func (n *Node) Info(w http.ResponseWriter, _ *http.Request) {
	type resp struct {
		PID   int       `json:"pid"`
		Now   time.Time `json:"now"`
		Items int       `json:"items"`
	}
	data, _ := json.Marshal(resp{PID: os.Getpid(), Now: time.Now(), Items: n.kv.Len()})
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(data)
	if err != nil {
		slog.Warn("Could not write http response header for info request")
	}
}

func (n *Node) replicaScheme() string {
	if n.serverTLS != nil {
		return "https"
	}
	return "http"
}

// forward forwards a http request to the Node that owns the key
func (n *Node) Forward(w http.ResponseWriter, req *http.Request, owner string) {
	if owner == "" {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	hostport := NormalizeHostPort(owner, "8080")
	if NormalizeHostPort(n.config.addr, "8080") == hostport {
		// last-resort safety; shouldn’t happen if handler compare is correct
		http.Error(w, "refusing to forward to self", http.StatusInternalServerError)
		return
	}
	target := *req.URL
	target.Scheme = n.replicaScheme()
	target.Host = hostport

	out, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	out.Header = req.Header.Clone()

	out.Header.Set("X-Forwarded-For", req.RemoteAddr)

	resp, err := n.replicaClient.Do(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		slog.Info("Error copying forwarded response")
	}
}

// put adds a key/value pair, writing to all replicas.
func (n *Node) Put(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]

	body, err := readBody(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ttl, err := parseTTL(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	replicaAddrs := n.ReplicasForKey(key)
	if len(replicaAddrs) == 0 {
		http.Error(w, "no owner or replicas for key", http.StatusServiceUnavailable)
		return
	}

	selfAddr := NormalizeHostPort(n.config.addr, "8080")
	var wg sync.WaitGroup
	for _, repAddr := range replicaAddrs {
		wg.Add(1)
		go func(repAddr string) {
			defer wg.Done()
			if repAddr == selfAddr {
				slog.Info("[Writing Replica]", "url", req.URL.Path, "self addr", selfAddr)
				n.kv.Put(key, body, ttl)
			} else {
				slog.Info("[Forward PUT]", "key", key, "replica", repAddr, "self", selfAddr)
				replicaURL := n.replicaScheme() + "://" + repAddr + "/replica/" + key
				if q := req.URL.RawQuery; q != "" {
					replicaURL += "?" + q
				}
				repReq, err := http.NewRequestWithContext(req.Context(), http.MethodPut, replicaURL, bytes.NewReader(body))
				if err != nil {
					slog.Warn("error building replication request", "err", err)
					return
				}
				resp, err := n.replicaClient.Do(repReq)
				if err != nil {
					slog.Warn("error forwarding to replica", "err", err, "replica", repAddr)
					return
				}
				_ = resp.Body.Close()
			}
		}(repAddr)
	}

	wg.Wait()
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
		slog.Info("[Forward GET]", "key", key, "owner", owner, "self", self)
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
	_, err := w.Write(val)
	if err != nil {
		slog.Info("Error writing response for GET request")
	}
}

// del removes a key
func (n *Node) Del(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	replicaAddrs := n.ReplicasForKey(key)
	if len(replicaAddrs) == 0 {
		http.Error(w, "no owner or replicas for key", http.StatusServiceUnavailable)
		return
	}

	body, err := readBody(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	selfAddr := NormalizeHostPort(n.config.addr, "8080")
	var wg sync.WaitGroup
	for _, addr := range replicaAddrs {
		wg.Add(1)
		go func(repAddr string) {
			defer wg.Done()
			if repAddr == selfAddr {
				// handle local case
				slog.Info("[Deleting Replica]", "url", req.URL.Path, "self addr", selfAddr)
				n.kv.Delete(key)
			} else {
				slog.Info("[Forward DEL]", "key", key, "replica", repAddr, "self", selfAddr)
				replicaURL := n.replicaScheme() + "://" + repAddr + "/replica/" + key

				repReq, err := http.NewRequestWithContext(req.Context(), http.MethodDelete, replicaURL, bytes.NewReader(body))
				if err != nil {
					slog.Warn("error building delete replica request", "err", err)
					return
				}
				resp, err := n.replicaClient.Do(repReq)
				if err != nil {
					slog.Warn("error forwarding delete to replica", "err", err, "replica", repAddr)
					return
				}
				_ = resp.Body.Close()
			}
		}(addr)
	}
	wg.Wait()
	w.WriteHeader(http.StatusNoContent)
}
