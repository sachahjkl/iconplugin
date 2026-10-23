// Command preview serves the generated gallery over HTTP for screenshots and manual review.
// It is a development helper, not part of the MCP server.
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8791", "listen address")
	dir := flag.String("dir", "demo", "directory to serve")
	flag.Parse()
	log.Printf("serving %s on http://%s/", *dir, *addr)
	log.Fatal(http.ListenAndServe(*addr, http.FileServer(http.Dir(*dir))))
}
