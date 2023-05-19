package main

import (
	"flag"
	"fmt"
	"github.com/mailsac/dracula/server"
	"os"
	"strings"
	"sync"
)

var (
	help            = flag.Bool("h", false, "Print this help")
	expireAfterSecs = flag.Int64("t", 60, "TTL secs - entries will expire after this many seconds")
	port            = flag.Int("p", 3509, "UDP this server will run on")
	tcpPort         = flag.Int("tcp", 3509, "TCP port this server will run on")
	secret          = flag.String("s", "", "Optional pre-shared auth secret if not using env var DRACULA_SECRET")
	peerIPPort      = flag.String("i", "", "Self peer IP and host like 192.168.0.1:3509 to identify self in the cluster")
	peers           = flag.String("c", "", "Enable cluster replication. Peers must be comma-separated ip:port like `192.168.0.1:3509,192.168.0.2:3555`.")
	verbose         = flag.Bool("v", false, "Verbose logging")
	printVersion    = flag.Bool("version", false, "Print version")
	promHostPort    = flag.String("prom", "", "Enable prometheus metrics. May cause pauses. Example: '0.0.0.0:9090'")
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
	var s *server.Server
	peerList := strings.Trim(*peers, " \n")
	if len(peerList) > 0 && *peerIPPort == "" {
		flag.Usage()
		fmt.Println("peer list and self peer ip:port are required together")
		os.Exit(1)
	}
	if len(peerList) > 0 {
		s = server.NewServerWithPeers(*expireAfterSecs, preSharedSecret, *peerIPPort, peerList)
		if *verbose {
			fmt.Printf("dracula server cluster mode enabled: self=%s; peers=%s \n", *peerIPPort, s.Peers())
		}
	} else {
		s = server.NewServer(*expireAfterSecs, preSharedSecret)
	}
	if *verbose {
		s.DebugEnable(fmt.Sprintf("udp:%d, tcp:%d", *port, *tcpPort))
	}
	err := s.Listen(*port, *tcpPort)
	if err != nil {
		fmt.Println("Dracula startup listen error", err)
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
