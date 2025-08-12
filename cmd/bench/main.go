
package main

import (
    "bytes"
    "flag"
    "fmt"
    "io"
    "math/rand"
    "net/http"
    "sync"
    "time"
)

func main() {
    addr := flag.String("addr", "http://localhost:8080", "server address")
    n := flag.Int("n", 5000, "requests")
    conc := flag.Int("c", 32, "concurrency")
    valSize := flag.Int("val", 128, "value size bytes")
    flag.Parse()

    client := &http.Client{Timeout: 5 * time.Second}
    wg := sync.WaitGroup{}
    start := time.Now()
    ch := make(chan int, *conc)

    for i := 0; i < *n; i++ {
        wg.Add(1)
        ch <- 1
        go func(i int) {
            defer wg.Done()
            key := fmt.Sprintf("k%d", i)
            payload := bytes.Repeat([]byte{byte(rand.Intn(255))}, *valSize)
            _, _ = client.Post(*addr+"/kv/"+key, "application/octet-stream", bytes.NewReader(payload))
            resp, _ := client.Get(*addr + "/kv/" + key)
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
