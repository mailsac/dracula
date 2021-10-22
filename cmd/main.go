package main

import (
	"flag"
	"fmt"
	"github.com/mailsac/dracula/server"
	"os"
	"sync"
)

var help = flag.Bool("h", false, "Print this help")
var expireAfterSecs = flag.Int64("t", 60, "TTL secs - entries will expire after this many seconds")
var port = flag.Int("p", 3509, "Port this server will run on")
var secret = flag.String("s", "", "Optional pre-shared auth secret")
var verbose = flag.Bool("v", false, "Verbose logging")

func main() {
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	s := server.NewServer(*expireAfterSecs, *secret)
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
