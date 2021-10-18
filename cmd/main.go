package main

import (
	"flag"
	"fmt"
	"github.com/mailsac/dracula/server"
	"os"
)

var help = flag.Bool("h", false, "Print this help")
var expireAfterSecs = flag.Int64("t", 60, "TTL secs - entries will expire after this many seconds")
var port = flag.Int("p", 3509, "Port this server will run on")

func main() {
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	s := server.NewServer(*expireAfterSecs)
	err := s.Listen(*port)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("TTL server is listening on port", *port,
		"with expiry after", *expireAfterSecs, "seconds")
	for {}
}
