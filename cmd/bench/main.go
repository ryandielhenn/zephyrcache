package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	addrs := flag.String("addrs", "http://localhost:8080", "comma-separated server addresses")
	n := flag.Int("n", 5000, "requests")
	conc := flag.Int("c", 32, "concurrency")
	valSize := flag.Int("val", 128, "value size bytes")
	flag.Parse()

	nodes := strings.Split(*addrs, ",")
	var counter atomic.Uint64

	client := &http.Client{Timeout: 5 * time.Second}
	wg := sync.WaitGroup{}
	start := time.Now()
	ch := make(chan int, *conc)

	for i := 0; i < *n; i++ {
		wg.Add(1)
		ch <- 1
		go func(i int) {
			defer wg.Done()
			addr := nodes[counter.Add(1)%uint64(len(nodes))]
			key := fmt.Sprintf("k%d", i)
			payload := bytes.Repeat([]byte{byte(rand.Intn(255))}, *valSize)
			_, _ = client.Post(addr+"/kv/"+key, "application/octet-stream", bytes.NewReader(payload))
			resp, _ := client.Get(addr + "/kv/" + key)
			if resp != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			<-ch
		}(i)
	}
	wg.Wait()
	dur := time.Since(start)
	fmt.Printf("Completed %d ops in %s (%.2f ops/s)\n", *n*2, dur, float64(*n*2)/dur.Seconds())
}
