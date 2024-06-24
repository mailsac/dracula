package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mailsac/dracula/client"
)

var (
	ipPortPairs  = flag.String("i", "127.0.0.1:3509", "List of one or more comma-separated (default) server <ip:port> to connect to")
	ns           = flag.String("n", "default", "Entry key namespace value")
	entryKey     = flag.String("k", "", "Required: entry key or pattern for keys mode")
	count        = flag.Bool("count", false, "Mode: Count items at entry key")
	put          = flag.Bool("put", false, "Mode: Put item at entry key")
	cmdKeys      = flag.Bool("keys", false, "Mode: list keys matching this pattern (TCP)")
	namespaces   = flag.Bool("namespaces", false, "Mode: list namespaces")
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
	totalModes := 0
	if *count {
		totalModes++
	}
	if *put {
		totalModes++
	}
	if *cmdKeys {
		totalModes++
	}

	if totalModes != 1 {
		flag.Usage()
		fmt.Println("either -put, -count, -keys is required")
		return
	}
	if *secret != "" {
		preSharedSecret = *secret
	}

	conf := client.Config{RemoteUDPIPPortList: *ipPortPairs, Timeout: time.Duration(*timeoutSecs) * time.Second, PreSharedKey: preSharedSecret}
	isTcp := *cmdKeys // todo more tcp commands
	if isTcp {
		conf.RemoteTCPIPPortList = conf.RemoteUDPIPPortList
		conf.RemoteUDPIPPortList = ""
	}
	c := client.NewClient(conf)
	if *verbose {
		c.DebugEnable(fmt.Sprintf("%d", *localPort))
	}
	if !isTcp {
		err := c.Listen(*localPort)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
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
	if *cmdKeys {
		keys, err := c.KeyMatch(*ns, *entryKey)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if len(keys) == 0 {
			fmt.Println("(no matched keys)")
		} else {
			for i, k := range keys {
				fmt.Printf("%d) %s\n", i+1, k)
			}
		}

		os.Exit(0)
	}
	if *namespaces {
		namespaceList, err := c.ListNamespaces()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if len(namespaceList) > 0 {
			for index, namespace := range namespaceList {
				fmt.Printf("%d) %s\n", index+1, namespace)
			}
		} else {
			fmt.Println("(no namespaces)")
		}
	}

	fmt.Println("no command matched")
	os.Exit(1)
}
