package main

import (
	"flag"
	"fmt"
	"github.com/mailsac/dracula/client"
	"os"
	"time"
)

var (
	ipPortPairs  = flag.String("i", "127.0.0.1:3509", "List of one or more comma-separated server <ip:port> to connect to")
	ns           = flag.String("n", "default", "Entry key namespace value")
	entryKey     = flag.String("k", "", "Required: entry key")
	count        = flag.Bool("count", false, "Mode: Count items at entry key")
	put          = flag.Bool("put", false, "Mode: Put item at entry key")
	secret       = flag.String("s", "", "Optional pre-shared auth secret if not using env var DRACULA_SECRET")
	localPort    = flag.Int("p", 3510, "Local client port to receive responses on")
	timeoutSecs  = flag.Int64("t", 6, "Request timeout in seconds")
	help         = flag.Bool("h", false, "Print help")
	verbose      = flag.Bool("v", false, "Verbose logging")
	printVersion = flag.Bool("version", false, "Print version")
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
	if *ns == "" {
		flag.Usage()
		fmt.Println("-n 'namespace' is required")
		return
	}
	if *entryKey == "" {
		flag.Usage()
		fmt.Println("-k 'entrykey' is required")
		return
	}
	validMode := (*count || *put) && !(*count && *put)
	if !validMode {
		flag.Usage()
		fmt.Println("either -put or -count is required")
		return
	}
	if *secret != "" {
		preSharedSecret = *secret
	}

	c := client.NewClient(*ipPortPairs, time.Duration(*timeoutSecs)*time.Second, preSharedSecret)
	if *verbose {
		c.DebugEnable(fmt.Sprintf("%d", *localPort))
	}
	err := c.Listen(*localPort)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if *count {
		total, err := c.Count(*ns, *entryKey)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println(total)
		os.Exit(0)
	}
	if *put {
		err := c.Put(*ns, *entryKey)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Println("no command matched")
	os.Exit(1)
}
