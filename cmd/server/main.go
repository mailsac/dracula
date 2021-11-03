package main

import (
	"flag"
	"fmt"
	"github.com/mailsac/dracula/server"
	"os"
	"sync"
)

var (
	help            = flag.Bool("h", false, "Print this help")
	expireAfterSecs = flag.Int64("t", 60, "TTL secs - entries will expire after this many seconds")
	port            = flag.Int("p", 3509, "Port this server will run on")
	secret          = flag.String("s", "", "Optional pre-shared auth secret if not using env var DRACULA_SECRET")
	verbose         = flag.Bool("v", false, "Verbose logging")
	printVersion    = flag.Bool("version", false, "Print version")
)

// Version should be replaced at build time
var Version = "unknown"
// Build should be replaced at build time
var Build = "unknown"

func main() {
	preSharedSecret := os.Getenv("DRACULA_SECRET")
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *printVersion {
		fmt.Println(Version, Build)
		return
	}
	if *secret != "" {
		preSharedSecret = *secret
	}
	s := server.NewServer(*expireAfterSecs, preSharedSecret)
	if *verbose {
		s.Debug = true
	}
	err := s.Listen(*port)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if *verbose {
		fmt.Println("will expire keys after", *expireAfterSecs, "seconds")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait() // wait forever without burning cpu
}
