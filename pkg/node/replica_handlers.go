package node

import (
	"log/slog"
	"net/http"
)

// putReplica writes the key/value pair to the local store (called by the primary).
func (n *Node) PutReplica(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/replica/"):]
	slog.Info("[Writing Replica]", "url", req.URL.Path, "self id", n.addr)

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

	n.kv.Put(key, body, ttl)
	w.WriteHeader(http.StatusNoContent)
}

func (n *Node) DelReplica(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/replica/"):]
	slog.Info("[Deleting Replica]", "url", req.URL.Path, "self id", n.addr)
	n.kv.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}
