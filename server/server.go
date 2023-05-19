package server

import (
	"errors"
	"github.com/mailsac/dracula/protocol"
	"github.com/mailsac/dracula/server/rawmessage"
	"github.com/mailsac/dracula/store"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const MinimumExpirySecs = 2

var (
	// ErrExpiryTooSmall means the server was attempted to be initialized with less than MinimumExpirySecs.
	// Values smaller than this are unreliable so they are not allowed.
	ErrExpiryTooSmall    = errors.New("dracula server expiry is too short")
	ErrServerAlreadyInit = errors.New("dracula server already initialized")
	ErrBadPeersFormat    = errors.New("dracula server peers must be comma separated string of ipaddress:port")
)

type Server struct {
	store             *store.Store
	StoreMetrics      *store.Metrics
	conn              *net.UDPConn
	tcpConn           *net.TCPListener
	disposed          bool
	preSharedKey      []byte
	expireAfterSecs   int64
	messageProcessing chan *rawmessage.RawMessage
	peers             []net.UDPAddr
	log               *log.Logger
}

func NewServerWithPeers(expireAfterSecs int64, preSharedKey, selfPeerHostPort, peerStringList string) *Server {
	s := NewServer(expireAfterSecs, preSharedKey)
	var peers []net.UDPAddr
	if len(peerStringList) > 0 {
		peerParts := strings.Split(peerStringList, ",")
		for _, peerHostPort := range peerParts {
			if peerHostPort == selfPeerHostPort {
				// skip adding self to cluster peer list, otherwise we'll double count to ourselves
				continue
			}
			hostPortParts := strings.Split(peerHostPort, ":")
			if len(hostPortParts) != 2 {
				panic(ErrBadPeersFormat)
			}
			ip := net.ParseIP(hostPortParts[0])
			if ip == nil {
				panic(ErrBadPeersFormat)
			}
			port, errBadNum := strconv.Atoi(hostPortParts[1])
			if errBadNum != nil {
				panic(ErrBadPeersFormat)
			}
			peers = append(peers, net.UDPAddr{
				IP:   ip,
				Port: port,
			})
		}
	}
	s.peers = peers
	return s
}
func NewServer(expireAfterSecs int64, preSharedKey string) *Server {
	if expireAfterSecs < MinimumExpirySecs {
		panic(ErrExpiryTooSmall)
	}
	psk := []byte(preSharedKey)
	st := store.NewStore(expireAfterSecs)
	serv := &Server{
		store:             st,
		StoreMetrics:      st.LastMetrics,
		preSharedKey:      psk,
		expireAfterSecs:   expireAfterSecs,
		messageProcessing: make(chan *rawmessage.RawMessage, runtime.NumCPU()),
		log:               log.New(os.Stdout, "", 0),
	}
	serv.DebugDisable()
	return serv
}

func (s *Server) DebugEnable(prefix string) {
	s.log.SetOutput(os.Stdout)
	s.log.SetPrefix(prefix + " ")
}

func (s *Server) DebugDisable() {
	s.log.SetOutput(ioutil.Discard)
}

func (s *Server) Listen(udpPort, tcpPort int) error {
	if s.conn != nil {
		return ErrServerAlreadyInit
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: udpPort,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		return err
	}
	s.conn = conn

	tcpConn, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: tcpPort,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		return err
	}
	s.tcpConn = tcpConn

	s.log.Printf("server listening udp+tcp %s\n", conn.LocalAddr().String())

	s.setupWorkers(runtime.NumCPU()) // as many workers as buffer size of channel

	go s.readUDPFrames()
	go s.ReadTCPFrames()
	return nil
}

func (s *Server) Close() error {
	if s.disposed {
		return nil
	}
	s.disposed = true

	s.store.DisableCleanup()
	close(s.messageProcessing)
	udpErr := s.conn.Close()
	tcpErr := s.tcpConn.Close()

	if udpErr != nil {
		return udpErr
	}
	if tcpErr != nil {
		return tcpErr
	}
	return nil
}

func (s *Server) readUDPFrames() {
	for {
		if s.disposed {
			break
		}
		message := make([]byte, protocol.PacketSize)
		_, remote, err := s.conn.ReadFromUDP(message[:])
		if err != nil {
			s.log.Println("server udp read error:", err)
			continue
		}
		s.messageProcessing <- &rawmessage.RawMessage{Message: message, Remote: remote}
	}
}

// ReadTCPFrames can be used by a dracula server OR client to accept and handle TCP connections,
// reading the protocol frames and passing them to a channel for processing.
func (s *Server) ReadTCPFrames() {
	for {
		if s.disposed {
			break
		}
		conn, err := s.tcpConn.AcceptTCP()
		if err != nil {
			s.log.Println("server tcp accept error:", err)
			continue
		}
		go s.handleTCPConnection(conn)
	}
}

func (s *Server) handleTCPConnection(conn *net.TCPConn) {
	defer conn.Close()
	var err error
	for {
		err = rawmessage.ReadOneTcpMessage(s.log, s.messageProcessing, conn)
		if err != nil {
			break
		}
	}
}

func (s *Server) worker(messages <-chan *rawmessage.RawMessage) {
	for m := range messages {
		message := m.Message
		remote := m.Remote
		maybeTcpClient := m.MaybeTcpClient
		packet, err := protocol.ParsePacket(message)
		if maybeTcpClient != nil {
			packet.RequestClient = maybeTcpClient
		}

		var resPacket *protocol.Packet
		respond := func() {
			if packet.RequestClient != nil {
				resPacket.RequestClient = packet.RequestClient
				s.respondOrLogErrorTCP(resPacket)
				return
			}
			s.respondOrLogError(remote, resPacket)
		}

		if err != nil {
			s.log.Println("server received BAD packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString(), err)
			resPacket = protocol.NewPacketFromParts(protocol.ResError, packet.MessageIDBytes, packet.Namespace, []byte(err.Error()), s.preSharedKey)
			respond()
			continue
		}
		err = packet.Validate(s.preSharedKey)
		if err != nil {
			s.log.Println("server got bad hash:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
			resPacket = protocol.NewPacketFromParts(protocol.ResError, packet.MessageIDBytes, packet.Namespace, []byte(err.Error()), s.preSharedKey)
			respond()
			continue
		}

		s.log.Println("server received packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())

		switch packet.Command {
		case protocol.CmdPutReplicate:
			// replications get Put() but don't respond or re-replicate
			s.store.Put(packet.NamespaceString(), packet.DataValueString())
			break
		case protocol.CmdPut:
			s.store.Put(packet.NamespaceString(), packet.DataValueString())
			resPacket = protocol.NewPacketFromParts(protocol.CmdPut, packet.MessageIDBytes, packet.Namespace, []byte{}, s.preSharedKey)
			respond()
			if len(s.peers) != 0 {
				// note that the packet is copied because it will be changed
				s.republish(*packet)
			}
			break
		case protocol.CmdCount:
			countInt := s.store.Count(packet.NamespaceString(), packet.DataValueString())
			if countInt > math.MaxUint32 {
				countInt = math.MaxUint32 // prevent overflow
			}
			c := uint32(countInt)
			resPacket = protocol.NewPacketFromParts(protocol.CmdCount, packet.MessageIDBytes, packet.Namespace, protocol.Uint32ToBytes(c), s.preSharedKey)
			respond()
			break
		case protocol.CmdCountNamespace:
			countInt := s.store.CountEntries(packet.NamespaceString())
			if countInt > math.MaxUint32 {
				countInt = math.MaxUint32 // prevent overflow
			}
			c := uint32(countInt)
			resPacket = protocol.NewPacketFromParts(protocol.CmdCountNamespace, packet.MessageIDBytes, packet.Namespace, protocol.Uint32ToBytes(c), s.preSharedKey)
			respond()
			break
		case protocol.CmdCountServer:
			countInt := s.store.CountServerEntries()
			if countInt > math.MaxUint32 {
				countInt = math.MaxUint32 // prevent overflow
			}
			c := uint32(countInt)
			resPacket = protocol.NewPacketFromParts(protocol.CmdCountServer, packet.MessageIDBytes, packet.Namespace, protocol.Uint32ToBytes(c), s.preSharedKey)
			respond()
			break
		case protocol.CmdTCPOnlyKeys:
			matchedKeys := s.store.KeyMatch(packet.NamespaceString(), packet.DataValueString())
			s.log.Println("KeyMatch", packet.NamespaceString(), packet.DataValueString(), matchedKeys)
			resPacket = protocol.NewPacketFromParts(protocol.CmdTCPOnlyKeys, packet.MessageIDBytes, packet.Namespace, []byte(strings.Join(matchedKeys, "\n")), s.preSharedKey)
			respond()
			break
		default:
			resPacket = protocol.NewPacketFromParts(protocol.ResError, packet.MessageIDBytes, packet.Namespace, []byte("unknown_command_"+string(packet.Command)), s.preSharedKey)
			respond()
			break
		}
	}
}

// republish changes the packet for republication and sends to all peers as an 'R' command packet.
func (s *Server) republish(packet protocol.Packet) {
	// re-hash the packet
	packet.Command = protocol.CmdPutReplicate
	packet.SetHash(s.preSharedKey)

	b, err := packet.Bytes()
	if err != nil {
		s.log.Println("server error: reconstructing replicant packet", err, packet.MessageID, packet.NamespaceString(), packet.DataValueString())
		return
	}

	for _, peer := range s.peers {
		_, err = s.conn.WriteToUDP(b, &peer)
		if err != nil {
			s.log.Println("server error: replicating to", peer, err, packet.MessageID, packet.NamespaceString(), packet.DataValueString())
			return
		}
		s.log.Println("server replicated to peer:", peer, packet.MessageID, packet.NamespaceString(), packet.DataValueString())
	}
}

func (s *Server) setupWorkers(numWorkers int) {
	for w := 0; w <= numWorkers; w++ {
		go s.worker(s.messageProcessing)
	}
}

func (s *Server) respondOrLogError(addr *net.UDPAddr, packet *protocol.Packet) {
	s.log.Println("server sending packet:", addr, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
	b, err := packet.Bytes()
	if err != nil {
		log.Println("server error: constructing packet for response", addr, err, packet)
		return
	}
	_, err = s.conn.WriteToUDP(b, addr)
	if err != nil {
		log.Println("server error: responding", addr, err, packet)
		return
	}
}

func (s *Server) respondOrLogErrorTCP(packet *protocol.Packet) {
	s.log.Println("server sending tcp res:", packet.RequestClient, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
	packet.DataValue = append(packet.DataValue, protocol.StopSymbol...)
	b, err := packet.Bytes()
	if err != nil && err != protocol.ErrBadOutputSize {
		log.Println("server error: constructing tcp res", packet.RequestClient, err, "|", string(b), "|")
		return
	}
	_, err = packet.RequestClient.Write(b)
	if err != nil {
		log.Println("server error: res from tcp write", packet.RequestClient, err, packet)
		return
	}
}

// Clear is for unit testing purposes. It will completely clear the data store.
func (s *Server) Clear() {
	s.store = store.NewStore(s.expireAfterSecs)
}

// Peers provides an informational notice about which peers this server will publish to, not including self
func (s *Server) Peers() string {
	var peers string
	for i, p := range s.peers {
		if i != 0 {
			peers += ","
		}
		peers += p.String()
	}
	return peers
}
