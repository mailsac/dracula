package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mailsac/dracula/server"
)

var (
	help            = flag.Bool("h", false, "Print this help")
	expireAfterSecs = flag.Int64("t", 60, "TTL secs - entries will expire after this many seconds")
	port            = flag.Int("p", 3509, "UDP this server will run on")
	tcpPort         = flag.Int("tcp", 3509, "TCP port this server will run on")
	restHostPort    = flag.String("http", "0.0.0.0:3510", "Enable HTTP REST interface. Example: '0.0.0.0:3510'")
	key             = flag.String("k", "", "Optional pre-shared auth secret key if not using env var DRACULA_SECRET")
	peerIPPort      = flag.String("i", "", "Self peer IP and host like 192.168.0.1:3509 to identify self in the cluster")
	peers           = flag.String("c", "", "Enable cluster replication. Peers must be comma-separated ip:port like `192.168.0.1:3509,192.168.0.2:3555`.")
	verbose         = flag.Bool("v", false, "Verbose logging")
	printVersion    = flag.Bool("version", false, "Print version")
	promHostPort    = flag.String("prom", "", "Enable prometheus metrics. May cause pauses. Example: '0.0.0.0:9090'")
	storage         = flag.String("s", "", "Set path to file location for persistent storage. Data will be stored in memopry if not set.")
)

// Version should be replaced at build time
var Version = "unknown"

// Build should be replaced at build time
var Build = "unknown"

func main() {
	preSharedSecret := os.Getenv("DRACULA_SECRET")
	storagePath := ""
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *printVersion {
		fmt.Println(Version, Build)
		return
	}
	if *key != "" {
		preSharedSecret = *key
	}
	if *storage != "" {
		storagePath = *storage
	}
	var s *server.Server
	peerList := strings.Trim(*peers, " \n")
	if len(peerList) > 0 && *peerIPPort == "" {
		flag.Usage()
		fmt.Println("peer list and self peer ip:port are required together")
		os.Exit(1)
	}
	if len(peerList) > 0 {
		s = server.NewServerWithPeers(*expireAfterSecs, preSharedSecret, *peerIPPort, peerList, storagePath)
		if *verbose {
			fmt.Printf("dracula server cluster mode enabled: self=%s; peers=%s \n", *peerIPPort, s.Peers())
		}
	} else {
		s = server.NewServer(*expireAfterSecs, preSharedSecret, storagePath)
	}
	if *verbose {
		s.DebugEnable(fmt.Sprintf("udp:%d, tcp:%d, http:%s -", *port, *tcpPort, *restHostPort))
	}
	err := s.Listen(*port, *tcpPort)
	if err != nil {
		fmt.Println("Dracula udp/tcp startup listen error", err)
		os.Exit(1)
	}
	err = s.ListenHTTP(*restHostPort)
	if err != nil {
		fmt.Println("Dracula http startup listen error", err)
		os.Exit(1)
	}
	if *verbose {
		fmt.Println("will expire keys after", *expireAfterSecs, "seconds")
	}
	if *promHostPort != "" {
		err = s.StoreMetrics.ListenAndServe(*promHostPort)
		if err != nil {
			fmt.Println("Prometheus startup listen error", *promHostPort, err)
			os.Exit(1)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait() // wait forever without burning cpu
}
